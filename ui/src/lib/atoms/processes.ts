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
    Env?: string[] | null;
    Cwd: string;
    Notify: { Mode: string; Interval: number };
    Timeout: number;
    IdleTimeout?: number;
    Terminal?: boolean;
    Columns?: number;
    Rows?: number;
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
  bytes?: Uint8Array;
}
export interface TerminalSnapshot {
  generation: number;
  chunks: TerminalChunk[];
  truncated: boolean;
}
export interface ProcessOutputEvent {
  process_id: string;
  stream: 'start' | 'stdout' | 'stderr' | 'exit' | 'control';
  terminal?: boolean;
  input?: boolean;
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
    ['start', 'stdout', 'stderr', 'exit', 'control'].includes(event.stream) &&
    (event.stream === 'control' || (Number.isSafeInteger(event.offset) && event.offset >= 0))
    ? event
    : null;
}

export function terminalChunkSize(chunk: TerminalChunk) {
  return chunk.bytes?.byteLength ?? chunk.text.length;
}
export function terminalBytes(chunks: TerminalChunk[]): Uint8Array {
  const encoded = chunks.map((chunk) => chunk.bytes ?? new TextEncoder().encode(chunk.text));
  const output = new Uint8Array(encoded.reduce((total, bytes) => total + bytes.length, 0));
  let position = 0;
  for (const bytes of encoded) {
    output.set(bytes, position);
    position += bytes.length;
  }
  return output;
}
