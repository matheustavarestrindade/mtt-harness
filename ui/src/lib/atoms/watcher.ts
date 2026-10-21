import type { Message } from './types';

export type WatcherTrigger = 'context_growth' | 'permission_request' | 'end_turn';
export interface ToolGrant {
  name: string;
  operations?: string[];
}
export interface WatcherTool {
  name: string;
  description: string;
  categories: string[];
  operations?: string[];
}
export interface WatcherDefinition {
  id: string;
  name: string;
  enabled: boolean;
  model: string;
  reasoning_effort: string;
  prompt: string;
  triggers: WatcherTrigger[];
  interval_tokens: number;
  context_tokens: number;
  include_reasoning: boolean;
  permission_tools: string[];
  outside_workspace_only: boolean;
  tools: ToolGrant[];
  timeout_ms: number;
  max_rounds: number;
  max_output_tokens: number;
  max_continuations: number;
}
export interface WatcherConfiguration {
  enabled: boolean;
  watchers: WatcherDefinition[];
}
export interface WatcherRun {
  continue_chat?: boolean;
  id: string;
  workspace_id: string;
  session_id: string;
  definition_id: string;
  name: string;
  trigger: WatcherTrigger;
  status: string;
  decision: string;
  message: string;
  reason: string;
  error: string;
  worker_session_id: string;
  model: string;
  created_at: string;
  finished_at: string;
  duration_ms: number;
  anchor_id: string;
  anchor_sequence: number;
  input_preview: string;
  input_truncated: boolean;
  input_tokens: number;
  estimated: boolean;
}
export interface WatcherRunPage {
  runs: WatcherRun[];
  next_cursor?: string;
}
export interface WatcherPreview {
  prompt: string;
  truncated: boolean;
  input_tokens: number;
  input_limit: number;
  estimated: boolean;
}
export const watcherTriggers: { value: WatcherTrigger; label: string }[] = [
  { value: 'context_growth', label: 'Context growth' },
  { value: 'permission_request', label: 'Permission request' },
  { value: 'end_turn', label: 'AI finish' },
];
export function watcherRuntimeIDs(message: Message): string[] {
  if (
    message.Role !== 'runtime' ||
    message.Content?.length !== 1 ||
    message.Content[0].Type !== 'text'
  )
    return [];
  const text = message.Content[0].Text ?? '';
  if (!text.startsWith('[watcher]\n')) return [];
  try {
    const value = JSON.parse(text.slice(10));
    if (
      value?.type !== 'watcher.intervention' ||
      !Array.isArray(value.run_ids) ||
      !value.run_ids.every((id: unknown) => typeof id === 'string') ||
      !Array.isArray(value.notes) ||
      !value.notes.every(
        (note: unknown) =>
          !!note &&
          typeof note === 'object' &&
          typeof (note as Record<string, unknown>).message === 'string',
      )
    )
      return [];
    return value.run_ids;
  } catch {
    return [];
  }
}
