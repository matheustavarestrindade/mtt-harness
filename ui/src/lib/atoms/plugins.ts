import type { MemoryMetrics, MemoryPluginState } from './memory';

export type WorkspacePluginState<Configuration> = Omit<
  MemoryPluginState,
  'Configuration' | 'Override'
> & { Configuration: Configuration; Override: Partial<Configuration> | null };

export type WorkspacePluginMetrics = MemoryMetrics;

export interface SidekickConfiguration {
  enabled: boolean;
  worker_model: string;
  worker_effort: string;
  memory_enabled: boolean;
  files_enabled: boolean;
  debounce_ms: number;
  cooldown_ms: number;
  job_timeout_ms: number;
  worker_count: number;
  worker_output_tokens: number;
  memory_limit: number;
  memory_bytes: number;
  file_limit: number;
  file_bytes: number;
  source_bytes: number;
  hint_bytes: number;
}

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
