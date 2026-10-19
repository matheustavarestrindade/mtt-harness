import type { HarnessEvent } from './types';

export interface ProcessExit {
  Code: number;
  Signal: string;
  Error: string;
}
export interface ProcessRecord {
  ID: string;
  InstanceID: string;
  SessionID: string;
  Spec: {
    Command: string;
    Args: string[] | null;
    Cwd: string;
    Notify: { Mode: string; Interval: number };
    Timeout: number;
    IdleTimeout?: number;
    ToolCallID?: string;
    ToolMessageID?: string;
  };
  PID: number;
  Status: string;
  Exit: ProcessExit | null;
  StartedAt: string;
  EndedAt: string;
}
export interface TerminalChunk {
  id: number;
  text: string;
}
export interface TerminalSnapshot {
  generation: number;
  chunks: TerminalChunk[];
  truncated: boolean;
}
export interface ProcessOutputEvent {
  process_id: string;
  stream: 'start' | 'stdout' | 'stderr' | 'exit';
  data?: string;
  data_base64?: string | null;
  offset: number;
  snapshot: boolean;
  at: string;
  error: string;
  exit?: ProcessExit | null;
}

export function processFromEvent(event: HarnessEvent): ProcessRecord | null {
  if (!['process.start', 'process.exit'].includes(event.Name)) return null;
  const value = event.Payload as Partial<ProcessRecord> | null;
  return value &&
    typeof value.ID === 'string' &&
    value.SessionID === event.SessionID &&
    value.Spec &&
    typeof value.Spec.Command === 'string'
    ? (value as ProcessRecord)
    : null;
}

export function processOutputEvent(value: unknown, identifier: string): ProcessOutputEvent | null {
  if (!value || typeof value !== 'object') return null;
  const event = value as ProcessOutputEvent;
  return event.process_id === identifier &&
    ['start', 'stdout', 'stderr', 'exit'].includes(event.stream) &&
    Number.isSafeInteger(event.offset) &&
    event.offset >= 0
    ? event
    : null;
}
