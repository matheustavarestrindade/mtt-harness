import { expect, test, type Page } from '@playwright/test';
import { mockHarness } from './api-fixture';

async function connectAndCreate(page: Page) {
  await page.goto('/');
  await page.getByRole('button', { name: 'Connect harness', exact: true }).click();
  await page.getByLabel('API token', { exact: true }).fill('test-token');
  await page.getByRole('button', { name: 'Connect to API' }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await page.getByRole('button', { name: 'Create workspace', exact: true }).click();
  await page.getByLabel('Project directory').fill('/workspace/example-project');
  await page
    .getByRole('dialog')
    .getByRole('button', { name: 'Create workspace', exact: true })
    .click();
  await page.getByRole('button', { name: 'Create session', exact: true }).click();
  await expect(page.getByLabel('Message', { exact: true })).toBeEnabled();
}

test('connect, create, chat and fit the viewport', async ({ page }, testInfo) => {
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await mockHarness(page);
  await connectAndCreate(page);
  await page.getByLabel('Message', { exact: true }).fill('Inspect the project');
  await page.getByRole('button', { name: 'Send message', exact: true }).click();
  await expect(page.getByText('Done. Inspect the project', { exact: true })).toBeVisible();
  await expect(page.getByLabel('Message', { exact: true })).toHaveValue('');
  await expect(page.getByLabel('Session usage')).toContainText('Unavailable');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
  const composer = await page.getByLabel('Message', { exact: true }).boundingBox();
  expect(composer).not.toBeNull();
  expect(composer!.y + composer!.height).toBeLessThanOrEqual(page.viewportSize()!.height);
  if (testInfo.project.name === 'mobile') {
    await page.getByRole('button', { name: 'Open navigation' }).click();
    await expect(page.getByRole('dialog')).toBeVisible();
    await page
      .getByRole('dialog')
      .getByRole('button', { name: /Session session-/ })
      .click();
    await expect(page.getByRole('dialog')).toHaveCount(0);
  }
  await page.screenshot({ path: testInfo.outputPath('console.png'), fullPage: true });
  expect(errors).toEqual([]);
});

test('accept messages during generation and cancel pending/current work', async ({ page }) => {
  await mockHarness(page);
  await connectAndCreate(page);
  await page.getByLabel('Message', { exact: true }).fill('Please wait for instructions');
  await page.getByRole('button', { name: 'Send message', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Stop', exact: true })).toBeVisible();
  await page.getByLabel('Message', { exact: true }).fill('A second request');
  await page.getByRole('button', { name: 'Queue message', exact: true }).click();
  await expect(page.getByRole('region', { name: 'Queued messages' })).toContainText('1 queued');
  await page.getByRole('button', { name: /Cancel queued message/ }).click();
  await expect(page.getByRole('region', { name: 'Queued messages' })).toHaveCount(0);
  await page.getByRole('button', { name: 'Stop', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Stop', exact: true })).toHaveCount(0);
});

test('show authentication failure and preserve the connection form', async ({ page }) => {
  await mockHarness(page);
  await page.goto('/');
  await page.getByRole('button', { name: 'Connect harness', exact: true }).click();
  await page.getByLabel('API token', { exact: true }).fill('incorrect-token');
  await page.getByRole('button', { name: 'Connect to API' }).click();
  await expect(page.getByRole('dialog')).toContainText('the token is not correct');
  await expect(page.getByRole('button', { name: 'Connect to API' })).toBeEnabled();
});

test('explain a missing workspace directory and preserve the form for correction', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  await page.route(
    (url) => url.pathname === '/api/instances',
    async (route) => {
      if (
        route.request().method() === 'POST' &&
        route.request().postDataJSON().workspace === '/teste'
      ) {
        return route.fulfill({
          status: 400,
          contentType: 'application/json',
          body: JSON.stringify({ error: 'stat /teste: no such file or directory' }),
        });
      }
      return route.fallback();
    },
  );
  await page.goto('/');
  await page.getByRole('button', { name: 'Connect harness', exact: true }).click();
  await page.getByLabel('API token', { exact: true }).fill('test-token');
  await page.getByRole('button', { name: 'Connect to API' }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await page.getByRole('button', { name: 'Create workspace', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog).toContainText('The folder must already exist.');
  await dialog.getByLabel('Project directory').fill('/teste');
  await dialog.getByRole('button', { name: 'Create workspace', exact: true }).click();
  await expect(dialog.getByRole('alert')).toContainText(
    'The directory "/teste" does not exist on the harness server.',
  );
  await expect(dialog.getByRole('alert')).toContainText('/workspace');
  await expect(dialog.getByLabel('Project directory')).toHaveValue('/teste');
  expect(fixture.instances).toHaveLength(0);
  await dialog.getByLabel('Project directory').fill('/workspace/existing-project');
  await dialog.getByRole('button', { name: 'Create workspace', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Create session', exact: true })).toBeVisible();
  expect(fixture.instances[0].Workspace).toBe('/workspace/existing-project');
});

test('render long model output as text and handle permission decisions', async ({ page }) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  await page.getByLabel('Message', { exact: true }).fill('wait');
  await page.getByRole('button', { name: 'Send message', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Stop', exact: true })).toBeVisible();
  fixture.event(fixture.sessions[0], 'permission.request', {
    ID: 'approval-1',
    InstanceID: fixture.instances[0].ID,
    SessionID: fixture.sessions[0].ID,
    Target: 'write:/workspace/other/file.go',
    Why: 'This path needs approval.',
  });
  await expect(page.getByRole('button', { name: 'Allow once' })).toBeVisible();
  await page.getByRole('button', { name: 'Allow once' }).click();
  await expect(page.getByRole('button', { name: 'Allow once' })).toHaveCount(0);
  await page.getByRole('button', { name: 'Stop', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Stop', exact: true })).toHaveCount(0);
  const content = '<img src=x onerror="window.injected=true">' + 'long-output/'.repeat(100);
  await page.getByLabel('Message', { exact: true }).fill(content);
  await page.getByRole('button', { name: 'Send message', exact: true }).click();
  await expect(page.getByRole('article', { name: 'assistant message' })).toContainText(content);
  expect(await page.evaluate(() => 'injected' in window)).toBe(false);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
});
