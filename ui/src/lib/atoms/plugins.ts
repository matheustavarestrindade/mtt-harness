import type { MemoryMetrics, MemoryPluginState } from './memory';

export type WorkspacePluginState<Configuration> = Omit<
  MemoryPluginState,
  'Configuration' | 'Override'
> & { Configuration: Configuration; Override: Partial<Configuration> | null };

export type WorkspacePluginMetrics = MemoryMetrics;

export interface RepetitionConfiguration {
  enabled: boolean;
  interval: {
    mode: 'tokens' | 'model_fraction';
    tokens: number;
    fraction: number;
    max_tokens: number;
  };
  pattern: ('low' | 'medium')[];
  worker_model: string;
  worker_effort: string;
  worker_output_tokens: number;
  job_timeout_ms: number;
  max_queries: number;
  memory_result_limit: number;
  memory_bytes: number;
  source_bytes: number;
  worker_count: number;
}
