import { expect, test } from '@playwright/test';
import { mockHarness } from './api-fixture';
import { connectAndCreate, openWorkbenchAction, workbenchReturnControl } from './helpers';

test('notifications leave panel controls reachable after a screen resize', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 960 });
  await mockHarness(page);
  await connectAndCreate(page);
  const notification = page.locator('[data-sonner-toast][data-front="true"]').first();
  await expect(notification).toBeVisible();
  await expect
    .poll(async () => (await notification.boundingBox())?.y ?? 0)
    .toBeGreaterThanOrEqual(120);
  await page.getByRole('button', { name: 'Memory settings', exact: true }).click({ timeout: 3000 });
  await expect(page.getByLabel('Worker model', { exact: true })).toBeVisible();
  await page
    .getByRole('button', { name: 'Close memory panel', exact: true })
    .click({ timeout: 3000 });
  await page.setViewportSize({ width: 390, height: 844 });
  await openWorkbenchAction(page, 'Memory');
  await expect(page.getByRole('dialog', { name: 'Workspace memory', exact: true })).toBeVisible();
});

test('keep the composer compact, grow with content and shrink after clearing', async ({
  page,
}, testInfo) => {
  await mockHarness(page);
  await connectAndCreate(page);
  const input = page.getByRole('textbox', { name: 'Message', exact: true });
  const height = () => input.evaluate((element) => element.getBoundingClientRect().height);
  await expect.poll(height).toBeLessThanOrEqual(48);
  const empty = await height();
  await input.fill('A short task');
  expect(await height()).toBe(empty);
  await input.fill(Array.from({ length: 6 }, (_, index) => `Line ${index + 1}`).join('\n'));
  await expect.poll(height).toBeGreaterThan(100);
  await input.fill('A long line that wraps on small screens. '.repeat(100));
  await expect.poll(height).toBeLessThanOrEqual(192);
  expect(await input.evaluate((element) => element.scrollHeight > element.clientHeight)).toBe(true);
  await input.fill('');
  await expect.poll(height).toBe(empty);
  await page.screenshot({ path: testInfo.outputPath('compact-composer.png'), fullPage: true });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test('collapse desktop navigation without losing drafts and restore its preference', async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name === 'mobile', 'Desktop navigation has a separate mobile drawer.');
  await mockHarness(page);
  await connectAndCreate(page);
  const input = page.getByRole('textbox', { name: 'Message', exact: true });
  await input.fill('Keep this draft');
  const before = await page.locator('#main-content').boundingBox();
  await page.getByRole('button', { name: 'Collapse navigation', exact: true }).click();
  await expect(page.getByRole('navigation', { name: 'Workspaces and sessions' })).toHaveCount(0);
  await expect(input).toHaveValue('Keep this draft');
  const expanded = await page.locator('#main-content').boundingBox();
  expect(expanded!.width - before!.width).toBeGreaterThan(250);
  await page.reload();
  await expect(page.getByRole('button', { name: 'Expand navigation', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Expand navigation', exact: true }).click();
  await expect(page.getByRole('navigation', { name: 'Workspaces and sessions' })).toBeVisible();
});

test('keep the compact controls and both drawers usable at 320 pixels', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 640 });
  await mockHarness(page);
  await connectAndCreate(page);
  await page
    .getByRole('textbox', { name: 'Message', exact: true })
    .fill('A draft at a narrow width');
  await openWorkbenchAction(page, 'Memory');
  await expect(page.getByRole('dialog', { name: 'Workspace memory', exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.getByRole('button', { name: 'Close memory panel', exact: true }).click();
  await expect(workbenchReturnControl(page, 'Open memory panel')).toBeFocused();
  await expect(page.getByRole('textbox', { name: 'Message', exact: true })).toHaveValue(
    'A draft at a narrow width',
  );
  await page.getByRole('button', { name: 'Open navigation', exact: true }).click();
  await expect(page.getByRole('dialog', { name: 'Workspaces and sessions' })).toBeVisible();
});

test('keep mobile actions in the drawer and hand off one panel at a time', async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await mockHarness(page);
  await connectAndCreate(page);
  const draft = page.getByLabel('Message', { exact: true });
  await draft.fill('Keep the mobile draft');
  const header = page.locator('#conversation-header');
  await expect(header.getByRole('button')).toHaveCount(1);
  await expect(header).toContainText('example-project');
  await expect(header).toContainText('test/test-model');
  await expect(header).not.toContainText('Session session-');

  await page.getByRole('button', { name: 'Open navigation', exact: true }).click();
  const navigation = page.getByRole('navigation', { name: 'Workspaces and sessions', exact: true });
  for (const label of ['New session', 'Tasks', 'Memory', 'Session activity', 'Settings']) {
    await expect(navigation.getByRole('button', { name: label, exact: true })).toBeInViewport();
  }
  await page.screenshot({
    path: testInfo.outputPath('mobile-navigation-actions.png'),
    fullPage: true,
  });

  await openWorkbenchAction(page, 'Tasks');
  await expect(page.getByRole('dialog', { name: 'Session tasks', exact: true })).toBeVisible();
  await expect(page.getByRole('dialog')).toHaveCount(1);
  await page.getByRole('button', { name: 'Close tasks panel', exact: true }).click();
  await expect(workbenchReturnControl(page, 'Open tasks panel')).toBeFocused();

  await openWorkbenchAction(page, 'Memory');
  await expect(page.getByRole('dialog', { name: 'Workspace memory', exact: true })).toBeVisible();
  await expect(page.getByRole('dialog')).toHaveCount(1);
  await page.keyboard.press('Escape');
  await expect(workbenchReturnControl(page, 'Open memory panel')).toBeFocused();

  for (const action of ['Session activity', 'Settings', 'New session'] as const) {
    await openWorkbenchAction(page, action);
    const dialog = page.getByRole('dialog');
    await expect(dialog).toHaveCount(1);
    await expect(
      dialog.getByRole('heading', {
        name: action === 'New session' ? 'Start a session' : action,
        exact: true,
      }),
    ).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(workbenchReturnControl(page, '')).toBeFocused();
  }
  await expect(draft).toHaveValue('Keep the mobile draft');
  await expect(page.getByRole('button', { name: 'Session settings', exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('mobile-compact-header.png'), fullPage: true });

  await page.setViewportSize({ width: 1440, height: 960 });
  for (const label of ['Open tasks panel', 'Open settings', 'Session activity', 'New session']) {
    await expect(header.getByRole('button', { name: label, exact: true })).toBeVisible();
  }
});
