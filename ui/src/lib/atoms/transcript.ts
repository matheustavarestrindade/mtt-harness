import type { Message, ToolCall } from './types';

export interface ToolActivity {
  call: ToolCall;
  result: Message | null;
}
export interface ConversationEntry {
  message: Message;
  tools: ToolActivity[];
}

// Match only preceding calls in the same session. Unmatched results remain visible.
// Reused call IDs in later turns must not consume an earlier result.
export function buildConversationEntries(messages: Message[]): ConversationEntry[] {
  const entries: ConversationEntry[] = [];
  const pending = new Map<string, ToolActivity>();
  for (const message of messages) {
    const resultKey = `${message.SessionID}:${message.ToolCallID}`;
    const activity = message.Role === 'tool' ? pending.get(resultKey) : undefined;
    if (activity && !activity.result) {
      activity.result = message;
      pending.delete(resultKey);
      continue;
    }
    const tools = (message.ToolCalls ?? []).map((call) => ({
      call,
      result: null as Message | null,
    }));
    entries.push({ message, tools });
    for (const tool of tools) pending.set(`${message.SessionID}:${tool.call.ID}`, tool);
  }
  return entries;
}
