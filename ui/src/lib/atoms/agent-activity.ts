import type { HarnessEvent, Message, ToolCall } from './types';

export type AgentPhase = 'unknown' | 'working' | 'waiting' | 'completed' | 'cancelled' | 'error';

export function eventPayload(event: HarnessEvent): Record<string, unknown> {
  return event.Payload && typeof event.Payload === 'object' && !Array.isArray(event.Payload)
    ? (event.Payload as Record<string, unknown>)
    : {};
}

export function eventToolCall(value: unknown): ToolCall | null {
  if (!value || typeof value !== 'object') return null;
  const call = value as Record<string, unknown>;
  return typeof call.ID === 'string' && typeof call.Name === 'string' && 'Input' in call
    ? (call as unknown as ToolCall)
    : null;
}

export function streamedMessage(event: HarnessEvent, messageID: string): Message {
  return {
    ID: messageID,
    SessionID: event.SessionID,
    Seq: 0,
    Role: 'assistant',
    Content: [],
    Reasoning: '',
    ToolCalls: null,
    ToolCallID: '',
    Usage: null,
    CreatedAt: event.Time,
  };
}

export function agentTask(call: ToolCall): string {
  if (!call.Input || typeof call.Input !== 'object') return '';
  const input = call.Input as Record<string, unknown>;
  return typeof input.task === 'string' ? input.task : '';
}

export function agentResult(call: ToolCall): string | null {
  if (call.Name !== 'finish' || !call.Input || typeof call.Input !== 'object') return null;
  const input = call.Input as Record<string, unknown>;
  return typeof input.result === 'string' ? input.result : null;
}
