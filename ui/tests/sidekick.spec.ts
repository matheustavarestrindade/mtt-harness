import { expect, test, type Page } from '@playwright/test';
import { mockHarness } from './api-fixture';
import { chooseSelectOption, connectAndCreate } from './helpers';

async function openSidekick(page: Page) {
  await page.getByRole('button', { name: 'Open settings', exact: true }).click();
  await page
    .getByRole('navigation', { name: 'Settings sections' })
    .getByRole('button', { name: 'Sidekick', exact: true })
    .click();
  const panel = page.getByRole('region', { name: 'Sidekick settings', exact: true });
  await expect(panel.getByRole('button', { name: 'Enable Sidekick', exact: true })).toBeVisible();
  return panel;
}

test('configure the real Sidekick fields without changing chat state', async ({
  page,
}, testInfo) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  await page.getByLabel('Message', { exact: true }).fill('Preserve this draft');
  const panel = await openSidekick(page);
  await panel.getByRole('button', { name: 'Enable Sidekick', exact: true }).click();
  await chooseSelectOption(panel.getByLabel('Worker model', { exact: true }), fixture.model.ID);
  await panel.getByLabel('Debounce (ms)', { exact: true }).fill('900');
  await panel.getByLabel('Cooldown (ms)', { exact: true }).fill('20000');
  await panel.getByRole('button', { name: 'Project files', exact: true }).click();
  await panel.getByRole('button', { name: 'Save and enable Sidekick', exact: true }).click();
  await expect(panel.getByRole('button', { name: 'Disable Sidekick', exact: true })).toBeVisible();
  expect(fixture.sidekickUpdates[0]).toEqual({
    workspaceID: fixture.instances[0].ID,
    patch: {
      worker_model: fixture.model.ID,
      worker_effort: '',
      memory_enabled: true,
      files_enabled: false,
      debounce_ms: 900,
      cooldown_ms: 20000,
      hint_bytes: 1600,
      enabled: true,
    },
  });
  await expect(page.getByLabel('Message', { exact: true })).toHaveValue('Preserve this draft');
  if (testInfo.project.name === 'mobile') await page.setViewportSize({ width: 320, height: 640 });
  expect(
    await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
  ).toBeTruthy();
  await page.screenshot({ path: testInfo.outputPath('sidekick-panel.png') });
});

test('validate retrieval sources and display observed activity', async ({ page }) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  fixture.sidekickMetrics.set(fixture.instances[0].ID, {
    Name: 'sidekick',
    WorkspaceID: fixture.instances[0].ID,
    Counters: {
      'hints/delivered': 7,
      'jobs/active': 1,
      'jobs/skipped': 4,
      'sources/memories': 8,
      'sources/files': 10,
    },
    Agents: [],
  });
  const panel = await openSidekick(page);
  await expect(panel.getByText('Notes delivered', { exact: true }).locator('..')).toContainText(
    '7',
  );
  await expect(panel.getByText('Worker cost', { exact: true }).locator('..')).toContainText('0.0');
  await panel.getByRole('button', { name: 'Configure Sidekick', exact: true }).click();
  await chooseSelectOption(panel.getByLabel('Worker model', { exact: true }), fixture.model.ID);
  await panel.getByRole('button', { name: 'Project files', exact: true }).click();
  await panel.getByRole('button', { name: 'Workspace memory', exact: true }).click();
  await panel.getByRole('button', { name: 'Save Sidekick settings', exact: true }).click();
  await expect(panel.getByRole('alert')).toContainText('at least one retrieval source');
  expect(fixture.sidekickUpdates).toHaveLength(0);
});

test('open Sidekick from the Tasks panel', async ({ page }) => {
  await mockHarness(page);
  await connectAndCreate(page);
  await page.getByRole('button', { name: 'Open tasks panel', exact: true }).click();
  await page.getByRole('button', { name: 'Sidekick settings', exact: true }).click();
  await expect(page.getByRole('region', { name: 'Sidekick settings', exact: true })).toBeVisible();
  await expect(page.getByRole('dialog')).toHaveCount(1);
});
