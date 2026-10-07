import type { Cost, Statistics } from './types';

// These shapes and counter names come from the implemented plugin API.
export interface MemoryConfiguration {
  enabled: boolean;
  worker_model: string;
  worker_effort: string;
  historian_model: string;
  [key: string]: unknown;
}
export interface MemoryPluginState {
  Name: string;
  Version: string;
  WorkspaceID: string;
  Available: boolean;
  Enabled: boolean;
  RequestedEnabled: boolean;
  Pending: boolean;
  Configuration: MemoryConfiguration;
  Override: Partial<MemoryConfiguration> | null;
  Error?: string;
}
export interface MemoryAgentUsage {
  Agent: string;
  ModelID: string;
  Statistics: Statistics;
  FailedCalls: number;
  DurationMilliseconds: number;
}
export interface MemoryMetrics {
  Name: string;
  WorkspaceID: string;
  Counters: Record<string, number>;
  Agents: MemoryAgentUsage[];
}

export function memoryCounter(metrics: MemoryMetrics | null, key: string): number {
  return metrics?.Counters[key] ?? 0;
}

export function memoryAgentLabel(name: string): string {
  return (
    (
      {
        'context.compactor': 'Compactor',
        'context.historian': 'Historian',
        'context.memory_writer': 'Memory extraction',
      } as Record<string, string>
    )[name] ?? name.replace(/^context\./, '').replaceAll('_', ' ')
  );
}

export function totalAgentUsage(agents: MemoryAgentUsage[]): {
  statistics: Statistics;
  unpricedCalls: number;
} {
  const statistics: Statistics = {
    Calls: 0,
    Input: 0,
    CacheRead: 0,
    CacheWrite: 0,
    Output: 0,
    Reasoning: 0,
    Cost: null,
    Costs: [],
    CacheHitRate: 0,
    CacheHitPercentage: 0,
  };
  const currencies = new Map<string, Cost>();
  let unpricedCalls = 0;
  for (const agent of agents) {
    const usage = agent.Statistics;
    for (const field of [
      'Calls',
      'Input',
      'CacheRead',
      'CacheWrite',
      'Output',
      'Reasoning',
    ] as const)
      statistics[field] += usage[field];
    const costs = usage.Costs?.length ? usage.Costs : usage.Cost ? [usage.Cost] : [];
    if (!costs.length) unpricedCalls += usage.Calls;
    for (const cost of costs) {
      const previous = currencies.get(cost.Currency);
      currencies.set(cost.Currency, {
        Currency: cost.Currency,
        Value: (previous?.Value ?? 0) + cost.Value,
        Estimated: Boolean(previous?.Estimated || cost.Estimated),
      });
    }
  }
  statistics.Costs = [...currencies.values()].sort((first, second) =>
    first.Currency.localeCompare(second.Currency),
  );
  statistics.Cost = statistics.Costs.length === 1 ? statistics.Costs[0] : null;
  const input = statistics.Input + statistics.CacheRead + statistics.CacheWrite;
  statistics.CacheHitRate = input ? statistics.CacheRead / input : 0;
  statistics.CacheHitPercentage = statistics.CacheHitRate * 100;
  return { statistics, unpricedCalls };
}

export function agentTokenCount(statistics: Statistics): number {
  // Provider output already includes reasoning; never add Reasoning again.
  return statistics.Input + statistics.CacheRead + statistics.CacheWrite + statistics.Output;
}

export function durationLabel(milliseconds: number): string {
  if (milliseconds < 1000) return `${milliseconds} ms`;
  if (milliseconds < 60_000) return `${(milliseconds / 1000).toFixed(1)} s`;
  const seconds = Math.round(milliseconds / 1000);
  return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
}
