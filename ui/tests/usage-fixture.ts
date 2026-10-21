import type { Page } from '@playwright/test';
import type { Statistics } from '../src/lib/atoms/types';
import type { UsageBucket, UsageReport } from '../src/lib/atoms/usage';

export function statistics(overrides: Partial<Statistics> = {}): Statistics {
  return {
    Calls: 1,
    Input: 0,
    Output: 0,
    CacheRead: 0,
    CacheWrite: 0,
    Reasoning: 0,
    Cost: null,
    Costs: [],
    CacheHitRate: 0,
    CacheHitPercentage: 0,
    ...overrides,
  };
}
export function usageBuckets(): UsageBucket[] {
  return [
    {
      Date: '2026-10-17',
      ModelID: 'provider-a/model-a',
      PricedCalls: 1,
      FailedCalls: 0,
      Statistics: statistics({
        Input: 1000,
        Output: 200,
        CacheRead: 800,
        Reasoning: 80,
        Cost: { Currency: 'USD', Value: 0.01 },
        Costs: [{ Currency: 'USD', Value: 0.01 }],
      }),
    },
    {
      Date: '2026-10-17',
      ModelID: 'provider-b/subscription',
      PricedCalls: 0,
      FailedCalls: 0,
      Statistics: statistics({ Input: 400, Output: 100 }),
    },
    {
      Date: '2026-10-18',
      ModelID: 'provider-a/model-a',
      PricedCalls: 1,
      FailedCalls: 0,
      Statistics: statistics({
        Input: 200,
        Output: 100,
        CacheRead: 50,
        Cost: { Currency: 'USD', Value: 0 },
        Costs: [{ Currency: 'USD', Value: 0 }],
      }),
    },
    {
      Date: '2026-10-19',
      ModelID: 'provider-a/model-a',
      PricedCalls: 1,
      FailedCalls: 0,
      Statistics: statistics({
        Input: 500,
        Output: 100,
        Cost: { Currency: 'EUR', Value: 0.2, Estimated: true },
        Costs: [{ Currency: 'EUR', Value: 0.2, Estimated: true }],
      }),
    },
    {
      Date: '2026-10-19',
      ModelID: 'legacy-model',
      PricedCalls: 0,
      FailedCalls: 1,
      Statistics: statistics(),
    },
  ];
}
export function reportFor(buckets: UsageBucket[], from: string, to: string): UsageReport {
  const selected = buckets.filter((bucket) => bucket.Date >= from && bucket.Date <= to);
  const total = statistics({ Calls: 0 }),
    costs = new Map<string, { Currency: string; Value: number; Estimated?: boolean }>();
  for (const bucket of selected) {
    for (const field of [
      'Calls',
      'Input',
      'Output',
      'CacheRead',
      'CacheWrite',
      'Reasoning',
    ] as const)
      total[field] += bucket.Statistics[field];
    for (const cost of bucket.Statistics.Costs?.length
      ? bucket.Statistics.Costs
      : bucket.Statistics.Cost
        ? [bucket.Statistics.Cost]
        : []) {
      const old = costs.get(cost.Currency);
      costs.set(cost.Currency, {
        Currency: cost.Currency,
        Value: (old?.Value ?? 0) + cost.Value,
        Estimated: !!(old?.Estimated || cost.Estimated),
      });
    }
  }
  total.Costs = [...costs.values()];
  total.Cost = total.Costs.length === 1 ? total.Costs[0] : null;
  return {
    From: from,
    To: to,
    Timezone: 'UTC',
    Statistics: total,
    PricedCalls: selected.reduce((sum, bucket) => sum + bucket.PricedCalls, 0),
    FailedCalls: selected.reduce((sum, bucket) => sum + bucket.FailedCalls, 0),
    Buckets: selected,
  };
}
export async function mockUsage(page: Page, buckets = usageBuckets()) {
  const queries: string[] = [];
  await page.route(
    (url) => url.pathname === '/api/usage',
    (route) => {
      const url = new URL(route.request().url());
      queries.push(url.search);
      const from = url.searchParams.get('from')!,
        to = url.searchParams.get('to')!;
      return route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify(reportFor(buckets, from, to)),
      });
    },
  );
  return { queries, buckets };
}
