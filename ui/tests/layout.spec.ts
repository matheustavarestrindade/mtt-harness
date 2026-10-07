import { expect, test } from '@playwright/test';
import { mockHarness } from './api-fixture';
import { connectAndCreate } from './helpers';

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
  await page
    .getByRole('button', { name: 'Open memory panel', exact: true })
    .click({ timeout: 3000 });
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
  await page.getByRole('button', { name: 'Open memory panel', exact: true }).click();
  await expect(page.getByRole('dialog', { name: 'Workspace memory', exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.getByRole('button', { name: 'Close memory panel', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Open memory panel', exact: true })).toBeFocused();
  await expect(page.getByRole('textbox', { name: 'Message', exact: true })).toHaveValue(
    'A draft at a narrow width',
  );
  await page.getByRole('button', { name: 'Open navigation', exact: true }).click();
  await expect(page.getByRole('dialog', { name: 'Workspaces and sessions' })).toBeVisible();
});
