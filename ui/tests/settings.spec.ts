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

  await openWorkbenchAction(page, 'Workspace settings');
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('tab', { name: 'Usage', exact: true }).click();
  await chooseSelectOption(dialog.getByLabel('Scope', { exact: true }), 'session');
  await expect(dialog.getByText('0.0', { exact: true })).toBeVisible();

  Object.assign(fixture.sessionUsage, { Calls: 1, Input: 20, Output: 12 });
  fixture.event(fixture.sessions[0], 'turn.end', {});
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
  await openWorkbenchAction(page, 'Workspace settings');
  const dialog = page.getByRole('dialog');
  await expect(
    dialog.getByRole('heading', { name: 'Workspace settings', exact: true }),
  ).toBeVisible();
  await dialog.getByRole('tab', { name: 'Usage', exact: true }).click();
  await expect(dialog.getByLabel('Scope', { exact: true })).toHaveAttribute(
    'data-value',
    'workspace',
  );
  await dialog.getByRole('button', { name: 'Usage details', exact: true }).click();
  const breakdown = dialog.getByLabel('Usage breakdown', { exact: true });
  await expect(breakdown.getByText('Input', { exact: true }).locator('..')).toContainText('1,000');
  await expect(breakdown.getByText('Cache read', { exact: true }).locator('..')).toContainText(
    '500',
  );
  await expect(breakdown.getByText('Cache write', { exact: true }).locator('..')).toContainText(
    '100',
  );
  await expect(breakdown.getByText('31.3%', { exact: true })).toBeVisible();
  await expect(
    dialog.getByLabel('Usage totals').getByText('$0.125', { exact: true }),
  ).toBeVisible();
  await page.keyboard.press('Escape');
  await openWorkbenchAction(page, 'Settings');
  await dialog.getByRole('tab', { name: 'Usage', exact: true }).click();
  await expect(dialog.getByLabel('Usage totals')).toContainText('$0.75');
  await expect(dialog.getByLabel('Usage totals')).toContainText('€0.50');
  if (testInfo.project.name === 'mobile') await page.setViewportSize({ width: 320, height: 700 });
  for (const button of await dialog
    .getByRole('navigation', { name: 'Settings sections' })
    .getByRole('tab')
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

test('save harness and workspace limits in flat sections and restore inheritance', async ({
  page,
}, testInfo) => {
  const fixture = await mockHarness(page);
  if (testInfo.project.name === 'mobile') await page.setViewportSize({ width: 320, height: 740 });
  await connectAndCreate(page);
  await openWorkbenchAction(page, 'Settings');
  const dialog = page.getByRole('dialog');
  const depth = dialog.getByLabel('Agent depth limit', { exact: true });
  await expect(depth).toHaveValue('2');
  const scopeBounds = (await dialog.locator('[data-settings-page-title]').boundingBox())!;
  for (const label of ['Agent depth limit', 'Process limit']) {
    const input = dialog.getByLabel(label, { exact: true }),
      form = dialog.getByRole('form', { name: `${label} settings`, exact: true });
    const appearance = await form.evaluate((element) => {
      const style = getComputedStyle(element);
      return {
        left: style.paddingLeft,
        right: style.paddingRight,
        borderLeft: style.borderLeftWidth,
        borderRight: style.borderRightWidth,
        radius: style.borderRadius,
      };
    });
    expect(appearance).toEqual({
      left: '0px',
      right: '0px',
      borderLeft: '0px',
      borderRight: '0px',
      radius: '0px',
    });
    expect(Math.abs((await input.boundingBox())!.x - scopeBounds.x)).toBeLessThanOrEqual(1);
  }
  await depth.fill('3');
  await depth
    .locator('xpath=ancestor::form')
    .getByRole('button', { name: 'Save', exact: true })
    .click();
  await expect.poll(() => fixture.settings.get('')?.agent_depth_limit).toBe('3');
  await page.keyboard.press('Escape');
  await openWorkbenchAction(page, 'Workspace settings');
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
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await dialog.getByRole('tab', { name: 'General', exact: true }).click();
  await page.screenshot({ path: testInfo.outputPath('general-flat-settings.png') });
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
  await page.route(
    (url) => url.pathname === '/api/usage' && url.searchParams.has('session_id'),
    (route) =>
      route.fulfill({
        status: 503,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'Usage temporarily unavailable' }),
      }),
  );
  await page.keyboard.press('Escape');
  await openWorkbenchAction(page, 'Workspace settings');
  await dialog.getByRole('tab', { name: 'Usage', exact: true }).click();
  await chooseSelectOption(dialog.getByLabel('Scope', { exact: true }), 'session');
  await expect(dialog.getByRole('alert')).toContainText('Usage temporarily unavailable');
  await chooseSelectOption(dialog.getByLabel('Scope', { exact: true }), 'workspace');
  await expect(dialog.getByRole('alert')).toHaveCount(0);
  await dialog.getByRole('button', { name: 'Usage details', exact: true }).click();
  await expect(
    dialog.getByLabel('Usage breakdown').getByText('Input', { exact: true }).locator('..'),
  ).toContainText('1,000');
});
