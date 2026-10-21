import { expect, test, type Locator } from '@playwright/test';
import { mockHarness } from './api-fixture';
import { mockWatchers } from './watcher-fixture';
import { connectAndCreate, openWorkbenchAction } from './helpers';

async function expectAlignedClose(dialog: Locator, title: string, closeLabel = 'Close settings') {
  const heading = dialog.getByRole('heading', { name: title, exact: true }),
    close = dialog.getByRole('button', { name: closeLabel, exact: true });
  const text = (await heading.boundingBox())!,
    button = (await close.boundingBox())!;
  expect(Math.abs(text.y + text.height / 2 - button.y - button.height / 2)).toBeLessThanOrEqual(1);
  expect(button.height).toBeGreaterThanOrEqual(44);
  expect(button.x).toBeGreaterThan(text.x + text.width);
}

test('Configuration separates global and workspace pages and retains model controls', async ({
  page,
}, testInfo) => {
  await mockHarness(page);
  await mockWatchers(page);
  await connectAndCreate(page);
  if (testInfo.project.name === 'mobile') await page.setViewportSize({ width: 320, height: 820 });
  const composer = page.getByRole('form', { name: 'Message composer', exact: true }),
    configuration = composer.getByRole('button', { name: 'Configuration', exact: true });
  await page.getByLabel('Message', { exact: true }).fill('Keep this draft');
  await configuration.click();
  const menu = page.getByRole('menu', { name: 'Configuration menu', exact: true });
  await expect(menu.getByRole('menuitem')).toHaveText([
    'Settings',
    'Workspace settings',
    'Model and thinking',
  ]);
  await menu.getByRole('menuitem', { name: 'Settings', exact: true }).click();
  let dialog = page.getByRole('dialog', { name: 'Settings', exact: true });
  await expect(dialog.getByRole('tablist', { name: 'Settings pages' }).getByRole('tab')).toHaveText(
    ['General', 'Usage', 'Providers', 'Connection'],
  );
  await expectAlignedClose(dialog, 'Settings');
  await expect(dialog.getByLabel('Settings scope')).toHaveCount(0);
  await dialog.getByRole('button', { name: 'Close settings', exact: true }).click();
  await expect(configuration).toBeFocused();
  await configuration.press('Enter');
  await menu.getByRole('menuitem', { name: 'Workspace settings', exact: true }).click();
  dialog = page.getByRole('dialog', { name: 'Workspace settings', exact: true });
  await expect(dialog.getByRole('tablist', { name: 'Settings pages' }).getByRole('tab')).toHaveText(
    ['General', 'Instructions', 'Sidekick', 'Watchers', 'Usage'],
  );
  await expectAlignedClose(dialog, 'Workspace settings');
  await expect(dialog).not.toContainText('example-project · Workspace settings');
  await dialog.getByRole('tab', { name: 'Usage', exact: true }).click();
  await expect(dialog.getByLabel('Scope', { exact: true })).toHaveAttribute(
    'data-value',
    'workspace',
  );
  await dialog.getByLabel('Scope', { exact: true }).click();
  await expect(page.getByRole('option')).toHaveText(['Workspace', 'Current session']);
  await page.keyboard.press('Escape');
  await page.keyboard.press('Escape');
  await expect(configuration).toBeFocused();
  await configuration.click();
  await menu.getByRole('menuitem', { name: 'Model and thinking', exact: true }).click();
  const session = page.getByRole('dialog', { name: 'Session settings', exact: true });
  await expectAlignedClose(session, 'Session settings', 'Close session settings');
  await expect(session.getByLabel('Session model', { exact: true })).toBeVisible();
  await session.getByRole('button', { name: 'Close session settings', exact: true }).click();
  await expect(configuration).toBeFocused();
  await expect(page.getByLabel('Message', { exact: true })).toHaveValue('Keep this draft');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test('workspace plugins share header switches with adjacent configuration gears', async ({
  page,
}, testInfo) => {
  await mockHarness(page);
  await mockWatchers(page);
  await connectAndCreate(page);
  if (testInfo.project.name === 'mobile') await page.setViewportSize({ width: 320, height: 900 });
  await openWorkbenchAction(page, 'Workspace settings');
  const dialog = page.getByRole('dialog'),
    tabs = dialog.getByRole('tablist', { name: 'Settings pages' });
  let baseline: { width: number; height: number } | undefined;
  for (const [name, toggle, gear] of [
    ['Sidekick', 'Sidekick enabled', 'Configure Sidekick'],
    ['Instructions', 'Instruction reminders enabled', 'Configure reminders'],
    ['Watchers', 'Watchers enabled', ''],
  ] as const) {
    await tabs.getByRole('tab', { name, exact: true }).click();
    const header = dialog.locator('[data-settings-page-title]'),
      controls = header.locator('[data-plugin-header-controls]'),
      switchControl = controls.getByRole('switch', { name: toggle, exact: true });
    await expect(switchControl).toBeVisible();
    const bounds = (await switchControl.boundingBox())!;
    if (baseline) expect({ width: bounds.width, height: bounds.height }).toEqual(baseline);
    else baseline = { width: bounds.width, height: bounds.height };
    if (gear) {
      const configure = controls.getByRole('button', { name: gear, exact: true }),
        button = (await configure.boundingBox())!;
      expect(button.x).toBeGreaterThanOrEqual(bounds.x + bounds.width);
      expect(
        Math.abs(button.y + button.height / 2 - bounds.y - bounds.height / 2),
      ).toBeLessThanOrEqual(1);
      expect(button.height).toBeGreaterThanOrEqual(44);
      await configure.click();
      await expect(configure).toHaveAttribute('aria-expanded', 'true');
    }
    await expect(dialog).not.toContainText('example-project · Workspace settings');
    await expect(
      dialog.getByRole('button', { name: /^(Enable Sidekick|Disable Sidekick|Turn on|Turn off)$/ }),
    ).toHaveCount(0);
    const heading = (await header.getByRole('heading', { name, exact: true }).boundingBox())!,
      controlBounds = (await controls.boundingBox())!;
    expect(heading.x + heading.width).toBeLessThanOrEqual(controlBounds.x);
    await page.screenshot({
      path: testInfo.outputPath(`workspace-${name.toLowerCase()}-controls.png`),
    });
  }
});
