import { expect, test } from '@playwright/test';
import { mockHarness } from './api-fixture';
import { connectAndCreate, chooseSelectOption, expectSelectOptions } from './helpers';

test('configure the session through its cog and confirm smaller context switches in one dialog', async ({
  page,
}, testInfo) => {
  const fixture = await mockHarness(page);
  fixture.model.Reasoning = true;
  fixture.model.ReasoningEfforts = ['low', 'high'];
  fixture.models.push(
    { ...fixture.model, ID: 'test/larger', Name: 'Larger model', ContextMax: 256000 },
    { ...fixture.model, ID: 'test/equal', Name: 'Equal model', ContextMax: 256000 },
    {
      ...fixture.model,
      ID: 'test/smaller',
      Name: 'Smaller model',
      ContextMax: 32000,
      ReasoningEfforts: ['low'],
    },
  );
  await connectAndCreate(page);
  const composer = page.getByLabel('Message', { exact: true });
  const model = page.getByLabel('Session model', { exact: true });
  const thinking = page.getByLabel('Thinking effort', { exact: true });
  await composer.fill('Keep this draft');
  const cog = page.getByRole('button', { name: 'Session settings', exact: true });
  await expect(model).toHaveCount(0);
  await expect(thinking).toHaveCount(0);
  await expect(cog).toHaveAttribute('aria-haspopup', 'dialog');
  await cog.click();
  await expect(page.getByRole('dialog', { name: 'Session settings', exact: true })).toBeVisible();
  await chooseSelectOption(thinking, 'high');
  await expect(thinking).toHaveAttribute('data-value', 'high');
  await chooseSelectOption(model, 'test/larger');
  await expect(model).toHaveAttribute('data-value', 'test/larger');
  await expect(page.getByRole('dialog')).toHaveCount(1);
  await chooseSelectOption(model, 'test/equal');
  await expect(model).toHaveAttribute('data-value', 'test/equal');
  await expect(page.getByRole('dialog')).toHaveCount(1);
  await chooseSelectOption(model, 'test/smaller');
  const confirmation = page.getByRole('dialog');
  await expect(confirmation).toContainText('conversation context will be compacted to fit');
  await expect(confirmation).toContainText('256,000 → 32,000');
  expect(fixture.sessions[0].Model).toBe('test/equal');
  await confirmation.getByRole('button', { name: 'Cancel', exact: true }).click();
  await expect(confirmation).toHaveCount(1);
  await expect(
    confirmation.getByRole('heading', { name: 'Session settings', exact: true }),
  ).toBeVisible();
  await expect(model).toHaveAttribute('data-value', 'test/equal');
  await chooseSelectOption(model, 'test/smaller');
  await confirmation.getByRole('button', { name: 'Switch and compact', exact: true }).click();
  await expect(confirmation).toHaveCount(1);
  await expect(model).toHaveAttribute('data-value', 'test/smaller');
  await expect(thinking).toHaveAttribute('data-value', '');
  await expectSelectOptions(thinking, ['Default', 'Low']);
  if (testInfo.project.name === 'mobile') await page.setViewportSize({ width: 320, height: 700 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('session-settings.png'), fullPage: true });
  await confirmation.getByRole('button', { name: 'Done', exact: true }).click();
  await expect(confirmation).toHaveCount(0);
  await expect(cog).toBeFocused();
  await expect(model).toHaveCount(0);
  await expect(thinking).toHaveCount(0);
  await expect(composer).toHaveValue('Keep this draft');
  await page.screenshot({ path: testInfo.outputPath('compact-composer.png'), fullPage: true });
  await page.reload();
  await page.getByRole('button', { name: 'Session settings', exact: true }).click();
  await expect(page.getByLabel('Session model', { exact: true })).toHaveAttribute(
    'data-value',
    'test/smaller',
  );
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog')).toHaveCount(0);
});

test('retain history and the selected model if a model change fails', async ({ page }) => {
  const fixture = await mockHarness(page);
  fixture.models.push({ ...fixture.model, ID: 'test/other', ContextMax: 256000 });
  await connectAndCreate(page);
  await page.getByLabel('Message', { exact: true }).fill('Keep the conversation');
  await page.getByRole('button', { name: 'Send message', exact: true }).click();
  await expect(page.getByRole('article', { name: 'assistant message' })).toContainText(
    'Done. Keep the conversation',
  );
  await page.route('**/api/sessions/*/model', (route) =>
    route.fulfill({
      status: 503,
      contentType: 'application/json',
      body: JSON.stringify({ error: 'Cannot change this model' }),
    }),
  );
  await page.getByRole('button', { name: 'Session settings', exact: true }).click();
  await chooseSelectOption(page.getByLabel('Session model', { exact: true }), 'test/other');
  await expect(page.getByText('Cannot change this model', { exact: true })).toBeVisible();
  await expect(page.getByLabel('Session model', { exact: true })).toHaveAttribute(
    'data-value',
    'test/test-model',
  );
  await page.keyboard.press('Escape');
  await expect(page.getByRole('article', { name: 'assistant message' })).toContainText(
    'Done. Keep the conversation',
  );
});
