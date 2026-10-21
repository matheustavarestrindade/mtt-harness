import { expect, test } from '@playwright/test';
import { mockHarness } from './api-fixture';
import { mockUsage, statistics, reportFor, usageBuckets } from './usage-fixture';
import { connectAndCreate, openWorkbenchAction, chooseSelectOption } from './helpers';
import type { UsageBucket } from '../src/lib/atoms/usage';

async function openUsage(page: import('@playwright/test').Page) {
  await connectAndCreate(page);
  await openWorkbenchAction(page, 'Workspace settings');
  await page
    .getByRole('tablist', { name: 'Settings pages' })
    .getByRole('tab', { name: 'Usage', exact: true })
    .click();
  const panel = page.getByRole('region', { name: 'Usage overview' });
  await expect(panel.getByRole('heading', { name: 'Daily tokens' })).toBeVisible();
  return panel;
}

test('keep chart and usage detail rows together without stacked section gaps', async ({
  page,
}, testInfo) => {
  await page.clock.setFixedTime(new Date('2026-10-19T12:00:00Z'));
  await mockHarness(page);
  await mockUsage(page);
  if (testInfo.project.name === 'mobile') await page.setViewportSize({ width: 320, height: 760 });
  const panel = await openUsage(page),
    chart = panel.getByRole('region', { name: 'By model', exact: true });
  const viewData = chart.getByRole('button', { name: 'View data', exact: true }),
    usageDetails = panel.getByRole('button', { name: 'Usage details', exact: true });
  await usageDetails.scrollIntoViewIfNeeded();
  const viewBounds = (await viewData.boundingBox())!,
    detailsBounds = (await usageDetails.boundingBox())!;
  expect(detailsBounds.y - (viewBounds.y + viewBounds.height)).toBeLessThanOrEqual(4);
  expect(viewBounds.height).toBeGreaterThanOrEqual(testInfo.project.name === 'mobile' ? 44 : 28);
  expect(detailsBounds.height).toBeGreaterThanOrEqual(testInfo.project.name === 'mobile' ? 44 : 28);
  await viewData.focus();
  await page.keyboard.press('Tab');
  await expect(usageDetails).toBeFocused();
  await usageDetails.click();
  await expect(panel.getByLabel('Usage breakdown')).toBeVisible();
  await usageDetails.click();
  await viewData.click();
  await expect(chart.getByRole('table')).toBeVisible();
});

test('keep request counts integral while preserving fractional cost axes', async ({ page }) => {
  await page.clock.setFixedTime(new Date('2026-10-19T12:00:00Z'));
  await mockHarness(page);
  await mockUsage(page, [
    {
      Date: '2026-10-19',
      ModelID: 'synthetic/model',
      PricedCalls: 1,
      FailedCalls: 0,
      Statistics: statistics({
        Input: 1,
        Cost: { Currency: 'USD', Value: 0.004 },
        Costs: [{ Currency: 'USD', Value: 0.004 }],
      }),
    },
  ]);
  const panel = await openUsage(page);
  await panel.getByRole('tab', { name: 'Requests', exact: true }).click();
  const requests = panel.getByRole('region', { name: 'Daily requests', exact: true });
  await expect(requests.locator('svg').first()).toBeVisible();
  await expect.poll(() => requests.locator('.lc-text-tspan').allTextContents()).toContain('1');
  const tickLabels = await requests.locator('.lc-text-tspan').allTextContents();
  expect(tickLabels).toContain('1');
  expect(tickLabels.filter((label) => /^-?\d+\.\d+$/.test(label))).toEqual([]);
  await panel.getByRole('tab', { name: 'Cost', exact: true }).click();
  const cost = panel.getByRole('region', { name: 'Daily cost', exact: true });
  await expect(cost.locator('svg').first()).toBeVisible();
  await expect
    .poll(async () =>
      (await cost.locator('.lc-text-tspan').allTextContents()).some((label) => label === '0.001'),
    )
    .toBe(true);
});

test('render actual daily and model bars with separate cache and filter totals', async ({
  page,
}, testInfo) => {
  await page.clock.setFixedTime(new Date('2026-10-19T12:00:00Z'));
  await mockHarness(page);
  const fixture = await mockUsage(page);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const panel = await openUsage(page),
    daily = panel.getByRole('region', { name: 'Daily tokens', exact: true });
  await expect(daily.locator('svg').first()).toBeVisible();
  await expect
    .poll(() =>
      daily
        .locator('.lc-bars .lc-bar')
        .evaluateAll(
          (elements) =>
            elements.filter((element) => (element as SVGGraphicsElement).getBBox().height > 1)
              .length,
        ),
    )
    .toBeGreaterThan(0);
  await expect(panel.locator('[title="2,600"]')).toHaveText('2,600');
  const modelComparison = panel.getByRole('region', { name: 'By model', exact: true });
  await expect(modelComparison.getByRole('list', { name: 'By model categories' })).toContainText(
    'provider-a/model-a',
  );
  await expect(
    modelComparison.locator('.lc-axis-tick-label').filter({ hasText: /provider-/ }),
  ).toHaveCount(0);
  const marks = daily.locator('.lc-bars .lc-bar');
  const tallest = await marks.evaluateAll(
    (elements) =>
      elements
        .map((element, index) => ({
          index,
          height: (element as SVGGraphicsElement).getBBox().height,
        }))
        .sort((first, second) => second.height - first.height)[0].index,
  );
  await marks.nth(tallest).scrollIntoViewIfNeeded();
  const markBounds = (await marks.nth(tallest).boundingBox())!;
  if (testInfo.project.name === 'mobile')
    await page.touchscreen.tap(
      markBounds.x + markBounds.width / 2,
      markBounds.y + markBounds.height / 2,
    );
  else
    await page.mouse.move(
      markBounds.x + markBounds.width / 2,
      markBounds.y + markBounds.height / 2,
    );
  await expect(page.getByRole('tooltip')).toContainText('1,200');
  await page.mouse.move(0, 0);
  await daily.getByRole('button', { name: 'View data', exact: true }).click();
  const day = daily.getByRole('row').filter({ hasText: 'Oct 17, 2026' });
  await expect(day).toContainText('1,200');
  await expect(day).toContainText('500');
  await expect(daily.getByRole('row').filter({ hasText: 'Oct 16, 2026' })).toContainText('0');
  const requests = fixture.queries.length;
  await chooseSelectOption(panel.getByLabel('Provider', { exact: true }), 'provider-a');
  await expect(panel.locator('[title="2,100"]')).toHaveText('2,100');
  expect(fixture.queries.length).toBe(requests);
  await panel.getByRole('tab', { name: 'Cache', exact: true }).click();
  const cache = panel.getByRole('region', { name: 'Daily cache', exact: true });
  await cache.getByRole('button', { name: 'View data', exact: true }).click();
  await expect(cache.getByRole('row').filter({ hasText: 'Oct 17, 2026' })).toContainText('800');
  await expect(cache.getByRole('row').filter({ hasText: 'Oct 18, 2026' })).toContainText('50');
  if (testInfo.project.name === 'mobile') await page.setViewportSize({ width: 320, height: 760 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('usage-chart.png') });
  expect(errors).toEqual([]);
});

test('preserve currencies, estimated costs, explicit zero and missing subscription costs', async ({
  page,
}) => {
  await page.clock.setFixedTime(new Date('2026-10-19T12:00:00Z'));
  await mockHarness(page);
  await mockUsage(page);
  const panel = await openUsage(page);
  await panel.getByRole('tab', { name: 'Cost', exact: true }).click();
  await chooseSelectOption(panel.getByLabel('Currency', { exact: true }), 'USD');
  const daily = panel.getByRole('region', { name: 'Daily cost', exact: true });
  await daily.getByRole('button', { name: 'View data', exact: true }).click();
  await expect(daily.getByRole('row').filter({ hasText: 'Oct 18, 2026' })).toContainText(
    'USD 0.00',
  );
  await expect(daily.getByRole('row').filter({ hasText: 'Oct 17, 2026' })).toContainText(
    'Not reported',
  );
  await chooseSelectOption(panel.getByLabel('Currency', { exact: true }), 'EUR');
  await expect(daily).toContainText('includes estimates');
  await chooseSelectOption(panel.getByLabel('Provider', { exact: true }), 'provider-b');
  await expect(daily).toContainText('No cost reported for this selection.');
  await expect(panel.getByText('Unavailable', { exact: true })).toBeVisible();
});

test('use the shadcn calendar and preserve inclusive UTC preset dates', async ({ page }) => {
  await page.clock.setFixedTime(new Date('2026-10-19T23:30:00Z'));
  await mockHarness(page);
  const fixture = await mockUsage(page);
  const panel = await openUsage(page);
  await panel.getByRole('button', { name: 'Choose usage dates', exact: true }).click();
  await page.getByRole('button', { name: 'Last 7 days', exact: true }).click();
  await expect.poll(() => fixture.queries.at(-1)).toContain('from=2026-10-13');
  expect(fixture.queries.at(-1)).toContain('to=2026-10-19');
  await panel.getByRole('button', { name: 'Choose usage dates', exact: true }).click();
  const calendar = page.getByRole('application', { name: /^Usage dates/ });
  await expect(calendar).toBeVisible();
  const bounds = await calendar.boundingBox();
  expect(bounds!.x).toBeGreaterThanOrEqual(0);
  expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(page.viewportSize()!.width);
  await calendar.getByRole('button', { name: 'Saturday, October 17, 2026', exact: true }).click();
  await calendar.getByRole('button', { name: 'Sunday, October 18, 2026', exact: true }).click();
  const apply = page.getByRole('button', { name: 'Apply dates', exact: true });
  await expect(apply).toBeInViewport();
  await apply.click();
  await expect.poll(() => fixture.queries.at(-1)).toContain('from=2026-10-17');
  expect(fixture.queries.at(-1)).toContain('to=2026-10-18');
  await expect(panel.locator('[title="2,000"]')).toHaveText('2,000');
  await expect(panel).toBeVisible();
});

test('aggregate extra models without losing tokens and keep delayed responses scoped', async ({
  page,
}) => {
  await page.clock.setFixedTime(new Date('2026-10-19T12:00:00Z'));
  await mockHarness(page);
  const buckets: UsageBucket[] = Array.from({ length: 12 }, (_, index) => ({
    Date: '2026-10-19',
    ModelID: `provider/model-${index}`,
    Statistics: statistics({ Input: 10, Output: 10 }),
    PricedCalls: 0,
    FailedCalls: 0,
  }));
  await mockUsage(page, buckets);
  const panel = await openUsage(page);
  await expect(panel.locator('[title="240"]')).toHaveText('240');
  const daily = panel.getByRole('region', { name: 'Daily tokens', exact: true });
  await expect(daily.getByLabel('Daily tokens legend')).toContainText('Other models');
  await daily.getByRole('button', { name: 'View data', exact: true }).click();
  await expect(daily.getByRole('row').filter({ hasText: 'Oct 19, 2026' })).toContainText('100');
  let release!: () => void;
  const gate = new Promise<void>((resolve) => (release = resolve));
  let requested = false;
  await page.route(
    (url) => url.pathname === '/api/usage' && url.searchParams.has('session_id'),
    async (route) => {
      requested = true;
      await gate;
      try {
        await route.fulfill({
          contentType: 'application/json',
          body: JSON.stringify(reportFor(usageBuckets(), '2026-09-20', '2026-10-19')),
        });
      } catch {}
    },
  );
  await chooseSelectOption(panel.getByLabel('Scope', { exact: true }), 'session');
  await expect.poll(() => requested).toBe(true);
  await page.getByRole('button', { name: 'Close settings', exact: true }).click();
  await openWorkbenchAction(page, 'Settings');
  await page.getByRole('dialog').getByRole('tab', { name: 'Usage', exact: true }).click();
  await expect(panel.locator('[title="240"]')).toHaveText('240');
  release();
  await expect(panel.locator('[title="240"]')).toHaveText('240');
});
