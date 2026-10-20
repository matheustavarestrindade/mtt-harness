import type { Cost, Model, Statistics } from './types';

export type UsageMetric = 'tokens' | 'cost' | 'calls' | 'cache';
export interface UsageBucket {
  Date: string;
  ModelID: string;
  Statistics: Statistics;
  PricedCalls: number;
  FailedCalls: number;
}
export interface UsageReport {
  From: string;
  To: string;
  Timezone: 'UTC';
  Statistics: Statistics;
  PricedCalls: number;
  FailedCalls: number;
  Buckets: UsageBucket[];
}
export interface UsageQuery {
  from: string;
  to: string;
  instance_id?: string;
  session_id?: string;
  agent_prefix?: string;
}
export interface DateRange {
  from: string;
  to: string;
}
export interface ChartSeries {
  key: string;
  label: string;
  color: string;
}
export interface ChartDatum {
  id?: string;
  label: string;
  [key: string]: string | number | null | undefined;
}
export interface UsageModelRow {
  id: string;
  label: string;
  provider: string;
  statistics: Statistics;
  pricedCalls: number;
  failedCalls: number;
}
export const usageMetrics: { value: UsageMetric; label: string }[] = [
  { value: 'tokens', label: 'Tokens' },
  { value: 'cost', label: 'Cost' },
  { value: 'calls', label: 'Requests' },
  { value: 'cache', label: 'Cache' },
];
export const chartColors = Array.from({ length: 8 }, (_, index) => `var(--chart-${index + 1})`);

export function emptyStatistics(): Statistics {
  return {
    Calls: 0,
    Input: 0,
    Output: 0,
    CacheRead: 0,
    CacheWrite: 0,
    Reasoning: 0,
    Cost: null,
    Costs: [],
    CacheHitRate: 0,
    CacheHitPercentage: 0,
  };
}
export function combineStatistics(values: Statistics[]): Statistics {
  const result = emptyStatistics(),
    currencies = new Map<string, Cost>();
  for (const statistics of values) {
    for (const field of [
      'Calls',
      'Input',
      'Output',
      'CacheRead',
      'CacheWrite',
      'Reasoning',
    ] as const)
      result[field] += statistics[field];
    for (const cost of statistics.Costs?.length
      ? statistics.Costs
      : statistics.Cost
        ? [statistics.Cost]
        : []) {
      const previous = currencies.get(cost.Currency);
      currencies.set(cost.Currency, {
        Currency: cost.Currency,
        Value: (previous?.Value ?? 0) + cost.Value,
        Estimated: !!(previous?.Estimated || cost.Estimated),
      });
    }
  }
  result.Costs = [...currencies.values()].sort((first, second) =>
    first.Currency.localeCompare(second.Currency),
  );
  result.Cost = result.Costs.length === 1 ? result.Costs[0] : null;
  const allInput = result.Input + result.CacheRead + result.CacheWrite;
  result.CacheHitRate = allInput ? result.CacheRead / allInput : 0;
  result.CacheHitPercentage = result.CacheHitRate * 100;
  return result;
}
export function usageValue(
  statistics: Statistics,
  metric: UsageMetric,
  currency: string,
): number | null {
  if (metric === 'tokens') return statistics.Input + statistics.Output;
  if (metric === 'cache') return statistics.CacheRead + statistics.CacheWrite;
  if (metric === 'calls') return statistics.Calls;
  const cost = (
    statistics.Costs?.length ? statistics.Costs : statistics.Cost ? [statistics.Cost] : []
  ).find((value) => value.Currency === currency);
  return cost?.Value ?? (statistics.Calls === 0 ? 0 : null);
}
export function usageValueLabel(
  value: number | null,
  metric: UsageMetric,
  currency: string,
  compact = false,
): string {
  if (value === null) return 'Not reported';
  if (metric === 'cost')
    return `${currency} ${new Intl.NumberFormat(undefined, { minimumFractionDigits: 2, maximumFractionDigits: compact ? 2 : 6 }).format(value)}`;
  return new Intl.NumberFormat(
    undefined,
    compact ? { notation: 'compact', maximumFractionDigits: 1 } : {},
  ).format(value);
}
export function modelProvider(identifier: string): string {
  const position = identifier.indexOf('/');
  return position > 0 ? identifier.slice(0, position) : '';
}
export function usageModelLabel(identifier: string, models: Model[]): string {
  const matching = models.find((model) => model.ID === identifier);
  const provider = modelProvider(identifier);
  return matching?.Name
    ? `${matching.Name}${provider ? ` · ${provider}` : ''}`
    : identifier || 'Unknown model';
}
export function utcDay(date = new Date()): string {
  return date.toISOString().slice(0, 10);
}
export function recentUsageRange(days: number, today = utcDay()): DateRange {
  return {
    from: utcDay(new Date(Date.parse(`${today}T00:00:00Z`) - (days - 1) * 86400000)),
    to: today,
  };
}
export function usageDays(range: DateRange): string[] {
  const start = Date.parse(`${range.from}T00:00:00Z`),
    end = Date.parse(`${range.to}T00:00:00Z`);
  if (
    !Number.isFinite(start) ||
    !Number.isFinite(end) ||
    end < start ||
    (end - start) / 86400000 >= 366
  )
    return [];
  return Array.from({ length: Math.round((end - start) / 86400000) + 1 }, (_, index) =>
    utcDay(new Date(start + index * 86400000)),
  );
}
export function formatUsageDay(day: string, short = false): string {
  return new Intl.DateTimeFormat(undefined, {
    timeZone: 'UTC',
    month: 'short',
    day: 'numeric',
    ...(short ? {} : { year: 'numeric' }),
  }).format(new Date(`${day}T00:00:00Z`));
}
export function aggregateUsageModels(buckets: UsageBucket[], models: Model[]): UsageModelRow[] {
  const groups = new Map<string, UsageBucket[]>();
  for (const bucket of buckets) {
    const group = groups.get(bucket.ModelID) ?? [];
    group.push(bucket);
    groups.set(bucket.ModelID, group);
  }
  return [...groups.entries()].map(([id, group]) => ({
    id,
    label: usageModelLabel(id, models),
    provider: modelProvider(id),
    statistics: combineStatistics(group.map((bucket) => bucket.Statistics)),
    pricedCalls: group.reduce((sum, bucket) => sum + bucket.PricedCalls, 0),
    failedCalls: group.reduce((sum, bucket) => sum + bucket.FailedCalls, 0),
  }));
}
// Keep the chart bounded without dropping usage: every remaining model goes
// into Other. Model IDs never become CSS variable names or selectors.
export function usageTimeline(
  buckets: UsageBucket[],
  range: DateRange,
  models: Model[],
  metric: UsageMetric,
  currency: string,
): { data: ChartDatum[]; series: ChartSeries[] } {
  const rows = aggregateUsageModels(buckets, models).sort(
    (first, second) =>
      (usageValue(second.statistics, metric, currency) ?? -1) -
        (usageValue(first.statistics, metric, currency) ?? -1) || first.id.localeCompare(second.id),
  );
  const leading = rows.slice(0, 7),
    remaining = rows.slice(7),
    groups = leading.map((row) => [row.id]);
  if (remaining.length) groups.push(remaining.map((row) => row.id));
  const series: ChartSeries[] = groups.map((_, index) => ({
    key: `series${index}`,
    label: leading[index]?.label ?? 'Other models',
    color: chartColors[index],
  }));
  const indexByModel = new Map(
    groups.flatMap((identifiers, index) =>
      identifiers.map((identifier) => [identifier, index] as const),
    ),
  );
  const daily = new Map<string, Map<number, Statistics[]>>();
  for (const bucket of buckets) {
    const index = indexByModel.get(bucket.ModelID);
    if (index === undefined) continue;
    const day = daily.get(bucket.Date) ?? new Map<number, Statistics[]>();
    const values = day.get(index) ?? [];
    values.push(bucket.Statistics);
    day.set(index, values);
    daily.set(bucket.Date, day);
  }
  const data = usageDays(range).map((day) => {
    const result: ChartDatum = { label: day };
    for (let index = 0; index < series.length; index++)
      result[`series${index}`] = usageValue(
        combineStatistics(daily.get(day)?.get(index) ?? []),
        metric,
        currency,
      );
    return result;
  });
  return { data, series };
}
