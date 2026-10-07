import { expect, test, type Page } from '@playwright/test';
import type { MemoryMetrics } from '../src/lib/atoms/memory';
import { agentTokenCount, totalAgentUsage } from '../src/lib/atoms/memory';
import { mockHarness } from './api-fixture';
import { connectAndCreate, chooseSelectOption } from './helpers';

async function openMemory(page: Page) {
  const toggle = page.getByRole('button', { name: 'Open memory panel', exact: true });
  if (await toggle.isVisible()) await toggle.click();
  const panel = page.getByRole('region', { name: 'Workspace memory', exact: true });
  await expect(panel).toBeVisible();
  return panel;
}

function metrics(workspaceID: string): MemoryMetrics {
  return {
    Name: 'context',
    WorkspaceID: workspaceID,
    Counters: {
      'memory/active': 7,
      'memory/consolidated': 3,
      'memory/deleted': 2,
      'memory_version/active': 99,
      'source/': 82,
      'job/running': 1,
      'job/pending': 2,
      'job/ready': 3,
      'context.compactor/checkpoints': 4,
      'context.compactor/removed_messages': 16,
      'context.compactor/estimated_tokens_removed': 24000,
      'context.remember/saved_memories': 5,
    },
    Agents: [
      {
        Agent: 'context.compactor',
        ModelID: 'test/worker-model-with-a-long-descriptive-identifier',
        Statistics: {
          Calls: 3,
          Input: 1000,
          CacheRead: 300,
          CacheWrite: 200,
          Output: 500,
          Reasoning: 250,
          Cost: { Currency: 'USD', Value: 0.0125, Estimated: true },
          Costs: [{ Currency: 'USD', Value: 0.0125, Estimated: true }],
          CacheHitRate: 0.2,
          CacheHitPercentage: 20,
        },
        FailedCalls: 1,
        DurationMilliseconds: 2345,
      },
    ],
  };
}

test('show real memory counters and agent usage without double-counting versions or reasoning', async ({
  page,
}, testInfo) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const workspace = fixture.instances[0];
  fixture.memoryMetrics.set(workspace.ID, metrics(workspace.ID));
  fixture.memoryStates.set(workspace.ID, {
    Name: 'context',
    Version: '0.1.0',
    WorkspaceID: workspace.ID,
    Available: true,
    Enabled: true,
    RequestedEnabled: true,
    Pending: false,
    Configuration: {
      enabled: true,
      worker_model: fixture.model.ID,
      worker_effort: 'none',
      historian_model: '',
    },
    Override: {},
  });
  const panel = await openMemory(page);
  await panel.getByRole('button', { name: 'Refresh memory', exact: true }).click();
  const summary = panel.locator('[aria-label="Memory totals"]');
  await expect(summary).toContainText('10');
  await expect(summary).not.toContainText('99');
  await expect(summary).toContainText('2 retained in history');
  await expect(summary).toContainText('2,000');
  await expect(summary).toContainText('$0.0125');
  await expect(panel.getByRole('region', { name: 'Memory jobs' })).toContainText('Prepared');
  await panel.locator('summary').filter({ hasText: 'Compactor' }).click();
  await expect(panel.getByRole('region', { name: 'Memory agent usage' })).toContainText('1,500');
  await expect(panel.getByRole('region', { name: 'Memory agent usage' })).toContainText('2.3 s');
  await expect(panel.getByRole('region', { name: 'Memory activity' })).toContainText('24,000');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('memory-panel.png'), fullPage: true });
});

test('configure memory through the workspace API and show pending transitions', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const panel = await openMemory(page);
  await panel.getByRole('button', { name: 'Enable memory', exact: true }).click();
  await chooseSelectOption(panel.getByLabel('Worker model', { exact: true }), fixture.model.ID);
  await panel.getByRole('button', { name: 'Save and enable memory', exact: true }).click();
  await expect(panel.getByRole('status')).toHaveText('On');
  expect(fixture.memoryUpdates).toEqual([
    {
      workspaceID: fixture.instances[0].ID,
      patch: { worker_model: fixture.model.ID, worker_effort: '', enabled: true },
    },
  ]);
  await panel.getByRole('button', { name: 'Memory settings', exact: true }).click();
  fixture.memoryStates.get(fixture.instances[0].ID)!.Pending = true;
  await panel.getByRole('button', { name: 'Disable memory', exact: true }).click();
  await expect(panel.getByRole('status')).toHaveText('Turning off…');
  await expect(panel.getByRole('button', { name: 'Enable memory', exact: true })).toBeDisabled();
});

test('refresh live counts, handle missing prices, and keep API failures inside the memory panel', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const panel = await openMemory(page);
  const summary = panel.locator('[aria-label="Memory totals"]');
  await expect(summary).toContainText('0.0');
  const data = metrics(fixture.instances[0].ID);
  data.Agents[0].Statistics.Cost = null;
  data.Agents[0].Statistics.Costs = null;
  fixture.memoryMetrics.set(data.WorkspaceID, data);
  await panel.getByRole('button', { name: 'Refresh memory', exact: true }).click();
  await expect(summary).toContainText('Unavailable');
  data.Counters['memory/active'] = 12;
  await expect(summary.locator('dd').first()).toHaveText('15', { timeout: 10000 });
  await page.route('**/plugins/context/statistics', (route) =>
    route.fulfill({
      status: 503,
      contentType: 'application/json',
      body: JSON.stringify({ error: 'Memory metrics temporarily unavailable' }),
    }),
  );
  await panel.getByRole('button', { name: 'Refresh memory', exact: true }).click();
  await expect(panel.getByRole('alert')).toContainText('Memory metrics temporarily unavailable');
  await expect(summary.locator('dd').first()).toHaveText('15');
  await panel.getByRole('button', { name: 'Close memory panel', exact: true }).click();
  await expect(page.getByRole('textbox', { name: 'Message', exact: true })).toBeEnabled();
});

test('use a drawer on small screens and persist the desktop panel preference', async ({
  page,
}, testInfo) => {
  await mockHarness(page);
  await connectAndCreate(page);
  const panel = await openMemory(page);
  if (testInfo.project.name === 'mobile') {
    await expect(page.getByRole('dialog', { name: 'Workspace memory', exact: true })).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(panel).toHaveCount(0);
    await page.getByRole('button', { name: 'Open navigation', exact: true }).click();
    await expect(page.getByRole('dialog', { name: 'Workspaces and sessions' })).toBeVisible();
  } else {
    const before = await page.locator('#main-content').boundingBox();
    await panel.getByRole('button', { name: 'Close memory panel', exact: true }).click();
    expect(
      (await page.locator('#main-content').boundingBox())!.width - before!.width,
    ).toBeGreaterThan(280);
    await page.reload();
    await expect(
      page.getByRole('button', { name: 'Open memory panel', exact: true }),
    ).toBeVisible();
    await expect(page.getByRole('region', { name: 'Workspace memory', exact: true })).toHaveCount(
      0,
    );
  }
});

test('preserve explicit zero and mixed currencies in memory-agent totals', () => {
  const data = metrics('workspace');
  const first = data.Agents[0];
  const summary = totalAgentUsage([
    first,
    {
      ...first,
      Statistics: { ...first.Statistics, Costs: [{ Currency: 'EUR', Value: 0 }], Cost: null },
    },
  ]);
  expect(summary.statistics.Costs).toEqual([
    { Currency: 'EUR', Value: 0, Estimated: false },
    { Currency: 'USD', Value: 0.0125, Estimated: true },
  ]);
  expect(agentTokenCount(summary.statistics)).toBe(4000);
  expect(summary.unpricedCalls).toBe(0);
});
