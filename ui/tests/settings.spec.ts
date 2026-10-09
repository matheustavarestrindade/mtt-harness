import { expect, test } from '@playwright/test';
import { mockHarness } from './api-fixture';
import {
  connectAndCreate,
  chooseSelectOption,
  openWorkbenchAction,
  workbenchReturnControl,
} from './helpers';

test('show zero cost for a new chat while preserving unknown and reported costs', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  Object.assign(fixture.sessionUsage, { Calls: 0, Input: 0, Output: 0 });
  await connectAndCreate(page);
  const usage = page.getByLabel('Session usage', { exact: true });
  await expect(usage.getByText('0.0', { exact: true })).toBeVisible();
  await expect(usage).not.toContainText('Unavailable');

  await openWorkbenchAction(page, 'Settings');
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('button', { name: 'Usage', exact: true }).click();
  await expect(dialog.getByText('0.0', { exact: true })).toBeVisible();

  Object.assign(fixture.sessionUsage, { Calls: 1, Input: 20, Output: 12 });
  await dialog.getByRole('button', { name: 'Refresh usage', exact: true }).click();
  await expect(dialog.getByText('Unavailable', { exact: true })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(usage.getByText('Unavailable', { exact: true })).toBeVisible();

  fixture.sessionUsage.Cost = { Currency: 'USD', Value: 0 };
  fixture.sessionUsage.Costs = [fixture.sessionUsage.Cost];
  fixture.event(fixture.sessions[0], 'turn.end', {});
  await expect(usage.getByText('$0.00', { exact: true })).toBeVisible();
});

test('settings stay in a modal, preserve the draft, and show real usage scopes', async ({
  page,
}, testInfo) => {
  const failures: string[] = [];
  page.on('pageerror', (failure) => failures.push(failure.message));
  await mockHarness(page);
  await connectAndCreate(page);
  await page.getByLabel('Message', { exact: true }).fill('Keep this draft');
  await openWorkbenchAction(page, 'Settings');
  const dialog = page.getByRole('dialog');
  await expect(dialog.getByRole('heading', { name: 'Settings', exact: true })).toBeVisible();
  await dialog.getByRole('button', { name: 'Usage', exact: true }).click();
  await expect(dialog.getByLabel('Usage scope')).toHaveAttribute('data-value', 'session');
  await expect(dialog.getByRole('region', { name: 'Usage overview' })).toContainText('Unavailable');
  await chooseSelectOption(dialog.getByLabel('Usage scope'), 'workspace');
  await expect(dialog.getByText('1,600', { exact: true })).toBeVisible();
  await expect(dialog.getByText('31.3%', { exact: true })).toBeVisible();
  await expect(dialog.getByText('$0.125', { exact: true })).toBeVisible();
  await chooseSelectOption(dialog.getByLabel('Usage scope'), 'harness');
  await expect(dialog.getByText('$0.75 + €0.50', { exact: true })).toBeVisible();
  if (testInfo.project.name === 'mobile') await page.setViewportSize({ width: 320, height: 700 });
  for (const button of await dialog
    .getByRole('navigation', { name: 'Settings sections' })
    .getByRole('button')
    .all()) {
    await expect(button).toBeInViewport();
    expect((await button.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  }
  const bounds = await dialog.boundingBox();
  expect(bounds!.width).toBeLessThanOrEqual(page.viewportSize()!.width);
  expect(bounds!.height).toBeLessThanOrEqual(page.viewportSize()!.height);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('settings-usage.png'), fullPage: true });
  await page.keyboard.press('Escape');
  await expect(dialog).toHaveCount(0);
  await expect(page.getByLabel('Message', { exact: true })).toHaveValue('Keep this draft');
  await expect(workbenchReturnControl(page, 'Open settings')).toBeFocused();
  expect(failures).toEqual([]);
});

test('save harness and workspace limits and restore inheritance', async ({ page }) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  await openWorkbenchAction(page, 'Settings');
  const dialog = page.getByRole('dialog');
  const depth = dialog.getByLabel('Agent depth limit', { exact: true });
  await expect(depth).toHaveValue('2');
  await depth.fill('3');
  await depth
    .locator('xpath=ancestor::form')
    .getByRole('button', { name: 'Save', exact: true })
    .click();
  await expect.poll(() => fixture.settings.get('')?.agent_depth_limit).toBe('3');
  await chooseSelectOption(dialog.getByLabel('Settings scope'), 'workspace');
  await expect(depth).toHaveValue('');
  await expect(dialog).toContainText('Harness setting: 3');
  await depth.fill('0');
  const saveDepth = depth
    .locator('xpath=ancestor::form')
    .getByRole('button', { name: 'Save', exact: true });
  await saveDepth.click();
  await expect
    .poll(() => fixture.settings.get(fixture.instances[0].ID)?.agent_depth_limit)
    .toBe('0');
  await expect(saveDepth).toBeDisabled();
  await depth.fill('');
  await saveDepth.click();
  await expect
    .poll(() => fixture.settings.get(fixture.instances[0].ID)?.agent_depth_limit)
    .toBeUndefined();
  expect(fixture.settings.get('')?.agent_depth_limit).toBe('3');
  const processLimit = dialog.getByLabel('Process limit', { exact: true });
  await processLimit.fill('-1');
  await expect(
    processLimit.locator('xpath=ancestor::form').getByRole('button', { name: 'Save' }),
  ).toBeDisabled();
});

test('failed usage and settings requests remain visible and recover on retry', async ({ page }) => {
  await mockHarness(page);
  await connectAndCreate(page);
  let failed = true;
  await page.route(
    (url) => url.pathname === '/api/settings',
    (route) =>
      failed
        ? route.fulfill({
            status: 503,
            contentType: 'application/json',
            body: JSON.stringify({ error: 'Settings temporarily unavailable' }),
          })
        : route.fallback(),
  );
  await openWorkbenchAction(page, 'Settings');
  const dialog = page.getByRole('dialog');
  await expect(dialog.getByRole('alert')).toContainText('Settings temporarily unavailable');
  failed = false;
  await dialog.getByRole('button', { name: 'Retry', exact: true }).click();
  await expect(dialog.getByLabel('Agent depth limit', { exact: true })).toHaveValue('2');
  await page.route(/\/api\/sessions\/[^/]+\/statistics$/, (route) =>
    route.fulfill({
      status: 503,
      contentType: 'application/json',
      body: JSON.stringify({ error: 'Usage temporarily unavailable' }),
    }),
  );
  await dialog.getByRole('button', { name: 'Usage', exact: true }).click();
  await expect(dialog.getByRole('alert')).toContainText('Usage temporarily unavailable');
  await chooseSelectOption(dialog.getByLabel('Usage scope'), 'workspace');
  await expect(dialog.getByRole('alert')).toHaveCount(0);
  await expect(dialog.getByText('1,600', { exact: true })).toBeVisible();
});
