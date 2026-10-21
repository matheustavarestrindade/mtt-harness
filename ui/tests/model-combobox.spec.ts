import { expect, test } from '@playwright/test';
import { mockHarness } from './api-fixture';
import { chooseSelectOption, connectAndCreate, openSessionSettings } from './helpers';

test('group and search friendly model names while submitting provider-qualified IDs', async ({
  page,
}, testInfo) => {
  const fixture = await mockHarness(page);
  fixture.model.Name = 'Current model';
  fixture.models.push(
    { ...fixture.model, ID: 'openai/zeta-internal', Name: 'Zeta model' },
    { ...fixture.model, ID: 'openai/shared-internal', Name: 'Shared model' },
    { ...fixture.model, ID: 'deepseek/alpha-internal', Name: 'Alpha model', Tools: false },
    { ...fixture.model, ID: 'openai-codex/shared-internal', Name: 'Shared model' },
    { ...fixture.model, ID: 'openai-codex/fast-internal@priority', Name: 'Shared model (Fast)' },
  );
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await connectAndCreate(page);
  await openSessionSettings(page);
  const selector = page.getByRole('combobox', { name: 'Session model', exact: true });
  await expect(selector).toHaveAttribute('aria-haspopup', 'dialog');
  await selector.click();
  const search = page.getByRole('combobox', { name: 'Search models', exact: true });
  await expect(search).toBeFocused();
  const list = page.getByRole('listbox');
  await expect(list.locator('[data-slot="command-group"]')).toHaveCount(4);
  expect(
    await list
      .locator('[data-slot="command-group"]')
      .evaluateAll((groups) => groups.map((group) => group.getAttribute('aria-label'))),
  ).toEqual(['DeepSeek', 'OpenAI', 'OpenAI coding plan', 'Test provider']);
  await expect(
    list.getByRole('group', { name: 'OpenAI', exact: true }).getByRole('option'),
  ).toHaveText(['Shared model', 'Zeta model']);
  await expect(list.getByRole('option')).toHaveCount(6);
  await expect(list).not.toContainText('internal');
  await expect(list).not.toContainText('tool support');
  for (const option of await list.getByRole('option').all()) {
    expect((await option.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  }
  await page.screenshot({ path: testInfo.outputPath('provider-model-groups.png'), fullPage: true });
  await search.fill('coding plan');
  await expect(list.getByRole('option')).toHaveText(['Shared model', 'Shared model (Fast)']);
  await list.getByRole('option', { name: 'Shared model', exact: true }).click();
  await expect.poll(() => fixture.sessions[0].Model).toBe('openai-codex/shared-internal');
  await expect(selector).toHaveText('Shared model');
  await expect(selector).toBeFocused();
  await selector.press('ArrowDown');
  await search.fill('does-not-exist');
  await expect(page.getByText('No models found.', { exact: true })).toBeVisible();
  await search.fill('Alpha');
  await expect(list.getByRole('option')).toHaveText(['Alpha model']);
  await search.press('ArrowDown');
  await search.press('Enter');
  await expect.poll(() => fixture.sessions[0].Model).toBe('deepseek/alpha-internal');
  await expect(selector).toBeFocused();
  await selector.click();
  await page.keyboard.press('Escape');
  await expect(list).toHaveCount(0);
  await expect(page.getByRole('dialog', { name: 'Session settings', exact: true })).toBeVisible();
  await expect(selector).toBeFocused();
  expect(errors).toEqual([]);
});

test('keep the confirmed model during an API save and retain unavailable selections', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  fixture.models.push({ ...fixture.model, ID: 'other/new-id', Name: 'New model' });
  await connectAndCreate(page);
  let finishSave!: () => void;
  const saveGate = new Promise<void>((resolve) => {
    finishSave = resolve;
  });
  await page.route('**/api/sessions/*/model', async (route) => {
    await saveGate;
    await route.fulfill({
      status: 503,
      contentType: 'application/json',
      body: JSON.stringify({ error: 'Selection failed' }),
    });
  });
  await openSessionSettings(page);
  const selector = page.getByLabel('Session model', { exact: true });
  await chooseSelectOption(selector, 'other/new-id');
  await expect(selector).toBeDisabled();
  await expect(selector).toHaveAttribute('data-value', fixture.model.ID);
  finishSave();
  await expect(page.getByText('Selection failed', { exact: true })).toBeVisible();
  await expect(selector).toHaveAttribute('data-value', fixture.model.ID);
  await expect(selector).toBeFocused();
  fixture.sessions[0].Model = 'retired/no-longer-listed';
  await page.reload();
  await openSessionSettings(page);
  await expect(selector).toContainText('Unavailable');
  await selector.click();
  await expect(
    page.getByRole('option').and(page.locator('[data-option-value="retired/no-longer-listed"]')),
  ).toHaveAttribute('aria-disabled', 'true');
});

test('workspace creation uses a blank server path and no Docker or test-provider hints', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  await page.goto('/');
  await page.getByRole('button', { name: 'Connect harness', exact: true }).click();
  await page.getByLabel('API token', { exact: true }).fill('test-token');
  await page.getByRole('button', { name: 'Connect to API' }).click();
  await page.getByRole('dialog').waitFor({ state: 'hidden' });
  await page.getByRole('button', { name: 'Create workspace', exact: true }).click();
  const dialog = page.getByRole('dialog');
  const directory = dialog.getByLabel('Project directory');
  await expect(directory).toHaveValue('');
  await expect(directory).toHaveAttribute('placeholder', '/path/to/project');
  await expect(dialog).not.toContainText('Docker');
  await expect(dialog).not.toContainText('test/test-model');
  await directory.fill('~/projects/example');
  await expect(dialog.getByRole('alert')).toContainText('Home shortcuts (~) are not expanded');
  await expect(
    dialog.getByRole('button', { name: 'Create workspace', exact: true }),
  ).toBeDisabled();
  await directory.press('Enter');
  expect(fixture.instances).toHaveLength(0);
  await directory.fill('/home/example/projects/project');
  await dialog.getByRole('button', { name: 'Create workspace', exact: true }).click();
  await expect.poll(() => fixture.instances[0]?.Workspace).toBe('/home/example/projects/project');
  expect(fixture.instances[0].DefaultModel).toBe('test/test-model');
  await expect(page.getByRole('dialog').getByLabel('Model', { exact: true })).toHaveRole(
    'combobox',
  );
});
