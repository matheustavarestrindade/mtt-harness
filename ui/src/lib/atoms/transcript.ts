import type { Message, ToolCall } from './types';
import type { ProcessRecord } from './processes';
import { backgroundProcessReceipt, processNotice, type ProcessNotice } from './process-notices';

export interface ToolActivity {
  call: ToolCall;
  result: Message | null;
  processRecord?: ProcessRecord;
  processNotice?: ProcessNotice;
  relatedMessageIDs?: string[];
}
export interface ConversationEntry {
  message: Message;
  tools: ToolActivity[];
  processNotice?: ProcessNotice;
  processRecord?: ProcessRecord;
  relatedMessageIDs?: string[];
}

// Match only preceding calls in the same session. Unmatched results remain visible.
// Reused call IDs in later turns must not consume an earlier result.
export function buildConversationEntries(
  messages: Message[],
  processRecords: Iterable<ProcessRecord> = [],
): ConversationEntry[] {
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
    const tools: ToolActivity[] = (message.ToolCalls ?? []).map((call) => ({
      call,
      result: null as Message | null,
    }));
    entries.push({ message, tools });
    for (const tool of tools) pending.set(`${message.SessionID}:${tool.call.ID}`, tool);
  }
  return attachProcessNotices(entries, processRecords);
}

function attachProcessNotices(
  entries: ConversationEntry[],
  processRecords: Iterable<ProcessRecord>,
): ConversationEntry[] {
  const calls = new Map<string, ToolActivity>();
  const owners = new Map<string, ToolActivity | null>();
  const records = new Map<string, ProcessRecord>();
  for (const entry of entries) {
    for (const activity of entry.tools) {
      if (activity.call.Name !== 'bash') continue;
      calls.set(`${entry.message.SessionID}:${entry.message.ID}:${activity.call.ID}`, activity);
      const processID = backgroundProcessReceipt(activity.call, activity.result);
      if (processID) {
        const key = `${entry.message.SessionID}:${processID}`;
        owners.set(key, owners.has(key) ? null : activity);
      }
    }
  }
  for (const record of processRecords) {
    const key = `${record.SessionID}:${record.ID}`;
    records.set(key, record);
    const hasOrigin = Boolean(record.Spec.ToolMessageID || record.Spec.ToolCallID);
    const owner = hasOrigin
      ? calls.get(`${record.SessionID}:${record.Spec.ToolMessageID}:${record.Spec.ToolCallID}`)
      : owners.get(key);
    // Runtime origin fields take precedence, even when their call is outside a
    // bounded preview. A stale or ambiguous receipt must not override them.
    if (hasOrigin) owners.set(key, owner ?? null);
    if (owner) owner.processRecord = record;
  }
  const visible: ConversationEntry[] = [];
  const unlinked = new Map<string, ConversationEntry>();
  for (const entry of entries) {
    const notice = processNotice(entry.message);
    if (!notice) {
      visible.push(entry);
      continue;
    }
    const key = `${entry.message.SessionID}:${notice.processID}`;
    const owner = owners.get(key);
    if (owner) {
      (owner.relatedMessageIDs ??= []).push(entry.message.ID);
      if (owner.processNotice?.kind !== 'exit' || notice.kind === 'exit')
        owner.processNotice = notice;
      continue;
    }
    const previous = unlinked.get(key);
    if (previous) {
      (previous.relatedMessageIDs ??= []).push(entry.message.ID);
      if (previous.processNotice?.kind !== 'exit' || notice.kind === 'exit')
        previous.processNotice = notice;
      continue;
    }
    entry.processNotice = notice;
    entry.processRecord = records.get(key);
    unlinked.set(key, entry);
    visible.push(entry);
  }
  return visible;
}
