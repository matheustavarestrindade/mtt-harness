import { expect, test, type Page } from '@playwright/test';
import type { MemoryMetrics } from '../src/lib/atoms/memory';
import { agentTokenCount, totalAgentUsage } from '../src/lib/atoms/memory';
import { mockHarness } from './api-fixture';
import { connectAndCreate, chooseSelectOption, openWorkbenchAction } from './helpers';

async function openMemory(page: Page) {
  const panel = page.getByRole('region', { name: 'Workspace memory', exact: true });
  if (!(await panel.isVisible())) await openWorkbenchAction(page, 'Memory');
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

test('put Memory categories below full-width bars with hover and tap details', async ({
  page,
}, testInfo) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const usage = metrics(fixture.instances[0].ID);
  usage.Agents.push({
    ...usage.Agents[0],
    Agent: 'context.historian',
    ModelID: 'test/second-worker-model',
    Statistics: { ...usage.Agents[0].Statistics, Input: 60, Output: 30 },
  });
  fixture.memoryMetrics.set(fixture.instances[0].ID, usage);
  const widths = testInfo.project.name === 'desktop' ? [1280, 1600] : [390, 320];
  for (const [index, width] of widths.entries()) {
    await page.setViewportSize({ width, height: 900 });
    const panel = await openMemory(page);
    if (index === 0)
      await panel.getByRole('button', { name: 'Refresh memory', exact: true }).click();
    for (const title of ['Jobs by status', 'Tokens by model']) {
      const chart = panel.getByRole('region', { name: title, exact: true });
      await expect(chart.locator('[data-slot="chart"]')).toBeVisible();
      await expect
        .poll(
          () =>
            chart.evaluate((element) => {
              const container = element
                .querySelector('[data-slot="chart"]')!
                .getBoundingClientRect();
              const hitAreas = Array.from(element.querySelectorAll('.lc-tooltip-rect')).map(
                (mark) => mark.getBoundingClientRect().width,
              );
              return Math.max(0, ...hitAreas) / container.width;
            }),
          {
            message: `${title}: category labels must not reserve plot width at ${width}px`,
          },
        )
        .toBeGreaterThanOrEqual(0.88);
      const categories = chart.getByRole('list', { name: `${title} categories` });
      await expect(categories).toBeVisible();
      const expected =
        title === 'Tokens by model'
          ? usage.Agents.map((agent) => agent.ModelID)
          : ['Running', 'Queued', 'Prepared', 'Failed'];
      for (const label of expected) await expect(categories).toContainText(label);
      await expect(
        chart
          .locator('.lc-axis-tick-label')
          .filter({ hasText: /test\/|Running|Queued|Prepared|Failed/ }),
      ).toHaveCount(0);
      const labelBounds = (await categories.boundingBox())!,
        containerBounds = (await chart.locator('[data-slot="chart"]').boundingBox())!;
      expect(labelBounds.y).toBeGreaterThanOrEqual(containerBounds.y + containerBounds.height);
      expect(labelBounds.x).toBeGreaterThanOrEqual(containerBounds.x - 1);
      expect(labelBounds.x + labelBounds.width).toBeLessThanOrEqual(
        containerBounds.x + containerBounds.width + 1,
      );
      const colors = await categories
        .locator('li > span:first-child')
        .evaluateAll((elements) =>
          elements.map((element) => getComputedStyle(element).backgroundColor),
        );
      expect(new Set(colors).size).toBe(expected.length);
      const barColors = await chart
        .locator('.lc-bars .lc-bar')
        .evaluateAll((elements) =>
          elements
            .filter(
              (element) =>
                element.getBoundingClientRect().width > 0 &&
                element.getBoundingClientRect().height > 0,
            )
            .map((element) => getComputedStyle(element).fill),
        );
      expect(barColors.every((color) => colors.includes(color))).toBe(true);
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
  }
  const panel = page.getByRole('region', { name: 'Workspace memory', exact: true });
  const workers = panel.getByRole('region', { name: 'Tokens by model', exact: true });
  const mark = workers.locator('.lc-bars .lc-bar').first();
  await mark.scrollIntoViewIfNeeded();
  const bounds = (await mark.boundingBox())!;
  if (testInfo.project.name === 'mobile')
    await page.touchscreen.tap(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2);
  else await page.mouse.move(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2);
  await expect(page.getByRole('tooltip')).toContainText(
    'test/worker-model-with-a-long-descriptive-identifier',
  );
  await expect(page.getByRole('tooltip')).toContainText('1,500');
  await page.mouse.move(0, 0);
  await workers.getByRole('button', { name: 'View data', exact: true }).click();
  await expect(workers.getByRole('table')).toContainText(
    'test/worker-model-with-a-long-descriptive-identifier',
  );
  await expect(workers.getByRole('table')).toContainText('1,500');
  await page.screenshot({ path: testInfo.outputPath('memory-categories-below.png') });
});

test('show uncached worker input and output with cache separate and reasoning included once', async ({
  page,
}) => {
  const fixture = await mockHarness(page);
  await connectAndCreate(page);
  const value = metrics(fixture.instances[0].ID);
  value.Agents[0].Statistics = {
    Calls: 97,
    Input: 596336,
    CacheRead: 130560,
    CacheWrite: 0,
    Output: 54387,
    Reasoning: 13749,
    Cost: null,
    Costs: null,
    CacheHitRate: 130560 / 726896,
    CacheHitPercentage: (100 * 130560) / 726896,
  };
  value.Agents[0].FailedCalls = 3;
  value.Counters['context.compactor/estimated_tokens_removed'] = 752031;
  fixture.memoryMetrics.set(value.WorkspaceID, value);
  const panel = await openMemory(page);
  await panel.getByRole('button', { name: 'Refresh memory', exact: true }).click();
  await expect(panel.getByText('Reported worker tokens', { exact: true })).toBeVisible();
  await expect(panel.locator('[title="650,723"]')).toBeVisible();
  await expect(panel.getByText('97 calls · Workspace total', { exact: true })).toBeVisible();
  await panel.getByRole('button', { name: 'Worker details', exact: true }).click();
  await panel.locator('[data-slot="collapsible-trigger"]').filter({ hasText: 'Compactor' }).click();
  const details = panel.getByRole('region', { name: 'Memory agent usage' });
  await expect(details.getByText('596,336', { exact: true })).toBeVisible();
  await expect(details.getByText('54,387', { exact: true })).toBeVisible();
  await expect(details.getByText('Cache read', { exact: true }).locator('..')).toContainText(
    '130,560',
  );
  await expect(details.getByText('Cache write', { exact: true }).locator('..')).toContainText('0');
  await expect(details.getByText('Reasoning (included)', { exact: true })).toBeVisible();
  await panel.getByRole('button', { name: 'Activity details', exact: true }).click();
  await expect(panel.getByRole('region', { name: 'Memory activity' })).toContainText('≈ 752,031');
});

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
  await expect(summary).toContainText('1,500');
  await expect(summary).toContainText('$0.0125');
  await expect(panel.getByRole('region', { name: 'Memory jobs' })).toContainText('Prepared');
  await panel.getByRole('button', { name: 'Worker details', exact: true }).click();
  await panel.locator('[data-slot="collapsible-trigger"]').filter({ hasText: 'Compactor' }).click();
  const agentUsage = panel.getByRole('region', { name: 'Memory agent usage' });
  await expect(agentUsage.getByText('Input tokens', { exact: true }).locator('..')).toContainText(
    '1,000',
  );
  await expect(agentUsage.getByText('Cache read', { exact: true }).locator('..')).toContainText(
    '300',
  );
  await expect(agentUsage.getByText('Cache write', { exact: true }).locator('..')).toContainText(
    '200',
  );
  await expect(panel.getByRole('region', { name: 'Memory agent usage' })).toContainText('2.3 s');
  await panel.getByRole('button', { name: 'Activity details', exact: true }).click();
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
  expect(agentTokenCount(summary.statistics)).toBe(3000);
  expect(summary.statistics.CacheRead).toBe(600);
  expect(summary.statistics.CacheWrite).toBe(400);
  expect(summary.statistics.CacheHitPercentage).toBe(20);
  expect(agentTokenCount({ ...summary.statistics, Input: 0, Output: 0 })).toBe(0);
  expect(summary.unpricedCalls).toBe(0);
});
