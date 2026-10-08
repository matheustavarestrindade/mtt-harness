import { expect, test, type Page } from '@playwright/test';
import { mockHarness } from './api-fixture';
import { connectAndCreate } from './helpers';

async function openPanel(page: Page) {
  const button = page.getByRole('button', { name: 'Open memory panel', exact: true });
  if (await button.isVisible()) await button.click();
  const panel = page.getByRole('region', { name: 'Workspace memory', exact: true });
  await expect(panel).toBeVisible();
  return panel;
}

test('discard a late memory response after switching workspaces', async ({ page }, testInfo) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const original = fixture.instances[0];
  const other = { ...original, ID: 'second-workspace', Workspace: '/workspace/other-workspace' };
  fixture.instances.push(other);
  fixture.memoryMetrics.set(other.ID, {
    Name: 'context',
    WorkspaceID: other.ID,
    Counters: { 'memory/active': 22 },
    Agents: [],
  });
  let release = () => {};
  let started = () => {};
  const pending = new Promise<void>((resolve) => {
    release = resolve;
  });
  const received = new Promise<void>((resolve) => {
    started = resolve;
  });
  await page.route(`**/instances/${original.ID}/plugins/context/statistics`, async (route) => {
    started();
    await pending;
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        Name: 'context',
        WorkspaceID: original.ID,
        Counters: { 'memory/active': 111 },
        Agents: [],
      }),
    });
  });
  try {
    const panel = await openPanel(page);
    if (testInfo.project.name === 'desktop')
      await panel.getByRole('button', { name: 'Refresh memory', exact: true }).click();
    await received;
    if (testInfo.project.name === 'mobile') {
      await panel.getByRole('button', { name: 'Close memory panel', exact: true }).click();
      await page.getByRole('button', { name: 'Open navigation', exact: true }).click();
    }
    await page.getByRole('button', { name: 'Switch workspace', exact: true }).click();
    await page.getByRole('menuitem', { name: 'Refresh workspaces', exact: true }).click();
    await page.getByRole('menuitem', { name: 'other-workspace', exact: true }).click();
    if (testInfo.project.name === 'mobile')
      await page.getByRole('button', { name: 'Close navigation', exact: true }).click();
    const switched = await openPanel(page);
    await expect(switched.locator('[aria-label="Memory totals"] dd').first()).toHaveText('22');
    release();
    await expect(switched).not.toContainText('111');
    await expect(switched.getByText('other-workspace', { exact: true })).toBeVisible();
  } finally {
    release();
  }
});

test('handle null metrics and an unavailable plugin without an invented cost', async ({ page }) => {
  await mockHarness(page);
  await page.route('**/plugins/context/statistics', async (route) => {
    const workspaceID = new URL(route.request().url()).pathname.split('/')[3];
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        Name: 'context',
        WorkspaceID: workspaceID,
        Counters: null,
        Agents: null,
      }),
    });
  });
  await connectAndCreate(page);
  const panel = await openPanel(page);
  await expect(panel.locator('[aria-label="Memory totals"]')).toContainText('0.0');
  await page.route('**/plugins/context/settings', (route) =>
    route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: '{"error":"plugin is not registered"}',
    }),
  );
  await panel.getByRole('button', { name: 'Refresh memory', exact: true }).click();
  await expect(panel.getByRole('alert')).toContainText('Memory is not available on this harness.');
  await panel.getByRole('button', { name: 'Close memory panel', exact: true }).click();
  await expect(page.getByRole('textbox', { name: 'Message', exact: true })).toBeEnabled();
});

test('keep settings editable after a rejected update and preserve unchanged effort', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const workspaceID = fixture.instances[0].ID;
  fixture.memoryStates.set(workspaceID, {
    Name: 'context',
    Version: '0.1.0',
    WorkspaceID: workspaceID,
    Available: true,
    Enabled: false,
    RequestedEnabled: false,
    Pending: false,
    Configuration: {
      enabled: false,
      worker_model: fixture.model.ID,
      worker_effort: 'low',
      historian_model: '',
    },
    Override: {},
  });
  const panel = await openPanel(page);
  await panel.getByRole('button', { name: 'Refresh memory', exact: true }).click();
  await expect(panel.getByRole('status')).toHaveText('Off');
  await panel.getByRole('button', { name: 'Memory settings', exact: true }).click();
  await expect(panel.getByLabel('Worker model', { exact: true })).toHaveAttribute(
    'data-value',
    fixture.model.ID,
  );
  await panel.getByRole('button', { name: 'Save worker model', exact: true }).click();
  expect(fixture.memoryUpdates.at(-1)?.patch).toEqual({ worker_model: fixture.model.ID });
  expect(fixture.memoryStates.get(workspaceID)?.Configuration.worker_effort).toBe('low');
  await page.route('**/plugins/context/settings', async (route) => {
    if (route.request().method() !== 'PATCH') return route.fallback();
    return route.fulfill({
      status: 400,
      contentType: 'application/json',
      body: '{"error":"worker model is not available"}',
    });
  });
  await panel.getByRole('button', { name: 'Memory settings', exact: true }).click();
  await panel.getByRole('button', { name: 'Save worker model', exact: true }).click();
  await expect(panel.getByRole('alert')).toContainText('worker model is not available');
  await expect(panel.getByLabel('Worker model', { exact: true })).toBeEnabled();
});
