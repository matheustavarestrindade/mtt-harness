import type { PluginActivityEntry } from './plugin-activity';
import type { WatcherRun } from './watcher';
import { watcherTriggers } from './watcher';

export const watcherPreviewLimit = 100_000;

export function clipWatcherInput(input: string): string {
  const text = input.slice(0, watcherPreviewLimit);
  return /[\uD800-\uDBFF]$/.test(text) ? text.slice(0, -1) : text;
}
const eventOpen = '<watcher_event>';
const eventClose = '</watcher_event>';
const referenceNames = {
  todo: 'TODO',
  doing: 'DOING',
  trigger: 'Trigger',
  workspace: 'Workspace',
  permission_request: 'Permission request',
  tool_call: 'Tool call',
  conversation_to_analize: 'Conversation',
  conversation_to_analyze: 'Conversation',
} as const;

export interface WatcherReview {
  id: string;
  trigger: string;
  status: string;
  decision: string;
  feedback: string[];
  error: string;
  input: string;
  inputTruncated: boolean;
  createdAt: string;
  durationMS?: number;
  model?: string;
  inputTokens?: number;
  estimated: boolean;
  workerSessionID?: string;
  continueChat?: boolean;
}

export interface WatcherInputPart {
  kind: string;
  label: string;
  text: string;
  json: boolean;
  value: unknown;
  truncated: boolean;
}

export function watcherReviewFromActivity(entry: PluginActivityEntry): WatcherReview {
  return {
    id: entry.ID,
    trigger: entry.Trigger ?? '',
    status: entry.Outcome ?? '',
    decision: entry.Decision ?? '',
    feedback: (entry.Notes ?? []).map((note) => note.Text),
    error: entry.Error ?? '',
    input: entry.InputPreview ?? '',
    inputTruncated: entry.InputTruncated ?? false,
    createdAt: entry.CreatedAt,
    durationMS: entry.DurationMS,
    estimated: entry.Estimated,
    workerSessionID: entry.WorkerSessionID,
    continueChat: entry.ContinueChat,
  };
}

export function watcherReviewFromRun(run: WatcherRun): WatcherReview {
  return {
    id: run.id,
    trigger: run.trigger,
    status: run.status,
    decision: run.decision,
    feedback: run.message || run.reason ? [run.message || run.reason] : [],
    error: run.error,
    input: run.input_preview,
    inputTruncated: run.input_truncated,
    createdAt: run.created_at,
    durationMS: run.duration_ms,
    model: run.model,
    inputTokens: run.input_tokens,
    estimated: run.estimated,
    workerSessionID: run.worker_session_id,
    continueChat: run.continue_chat,
  };
}

export function watcherTriggerLabel(trigger: string): string {
  return watcherTriggers.find((value) => value.value === trigger)?.label ?? trigger;
}

export function watcherStatusLabel(status: string): string {
  return (
    (
      {
        running: 'Reviewing',
        reviewed: 'Reviewed',
        completed: 'Completed',
        intervened: 'Intervention delivered',
        user_input_requested: 'Needs your input',
        deferred_for_user: 'Held for user input',
        cancelled: 'Cancelled',
        interrupted: 'Interrupted',
        stale: 'Outdated',
        limited: 'Limit reached',
        error: 'Failed',
        failed: 'Failed',
        not_applied: 'Not applied',
        queued: 'Queued',
      } as Record<string, string>
    )[status] ?? status
  );
}

export function watcherReviewBecameInactive(
  previous: string | undefined,
  current: string | undefined,
): boolean {
  return (
    previous === 'running' &&
    [
      'reviewed',
      'completed',
      'intervened',
      'user_input_requested',
      'deferred_for_user',
      'cancelled',
      'interrupted',
      'stale',
      'limited',
      'error',
      'failed',
      'not_applied',
    ].includes(current ?? '')
  );
}

export function watcherObject(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

export function watcherEvent(value: unknown): Record<string, unknown> | null {
  const record = watcherObject(value);
  const trigger = record?.type ?? record?.kind;
  return typeof trigger === 'string' && watcherTriggers.some((item) => item.value === trigger)
    ? record
    : null;
}

// Completion events do not populate growth/model fields. A zero growth count is
// still data when the actual trigger is context growth, never a token-usage value.
export function watcherEventFields(event: Record<string, unknown>): [string, string][] {
  const trigger = String(event.type ?? event.kind);
  const fields: [string, string][] = [['Trigger', watcherTriggerLabel(trigger)]];
  const phases: Record<string, string> = {
    active_request: 'Active request',
    pending_permission: 'Pending permission',
    completed_turn: 'Completed turn',
  };
  if (typeof event.source_phase === 'string' && event.source_phase)
    fields.push(['Phase', phases[event.source_phase] ?? event.source_phase]);
  if (trigger === 'context_growth') {
    if (typeof event.context_tokens === 'number' && Number.isFinite(event.context_tokens))
      fields.push([
        event.context_basis === 'visible_input'
          ? 'Visible context at trigger'
          : 'Context at trigger',
        `${event.context_estimated === true ? '≈ ' : ''}${event.context_tokens.toLocaleString()} tokens`,
      ]);
    if (typeof event.source_active === 'boolean')
      fields.push(['Source active', event.source_active ? 'Yes' : 'No']);
  }
  if (typeof event.model === 'string' && event.model) fields.push(['Reviewed model', event.model]);
  if (typeof event.permission_id === 'string' && event.permission_id)
    fields.push(['Permission request', event.permission_id]);
  return fields;
}

function jsonValue(text: string): { json: boolean; value: unknown } {
  try {
    return { json: true, value: JSON.parse(text) };
  } catch {
    return { json: false, value: undefined };
  }
}

function fencedRanges(text: string): [number, number][] {
  const result: [number, number][] = [];
  const fences = /(?:^|\n)[ \t]{0,3}(`{3,}|~{3,})[^\n]*/g;
  let start: { index: number; marker: string } | undefined;
  for (const match of text.matchAll(fences)) {
    if (!start) start = { index: match.index!, marker: match[1] };
    else if (match[1][0] === start.marker[0] && match[1].length >= start.marker.length) {
      result.push([start.index, match.index! + match[0].length]);
      start = undefined;
    }
  }
  if (start) result.push([start.index, text.length]);
  return result;
}

export function parseWatcherInput(input: string, truncated = false) {
  let text = clipWatcherInput(input);
  const limited = text.length < input.length;
  let event: Record<string, unknown> | null = null;
  let eventSource = '';
  // Only the complete leading harness envelope is metadata. Quoted tags, code
  // examples, malformed data and unrelated text remain visible reference data.
  if (text.startsWith(eventOpen)) {
    const end = text.indexOf(eventClose, eventOpen.length);
    if (end >= 0) {
      eventSource = text.slice(eventOpen.length, end);
      event = watcherEvent(jsonValue(eventSource).value);
      if (event) text = text.slice(end + eventClose.length).trimStart();
    }
  }
  const parts: WatcherInputPart[] = [];
  const addText = (value: string) => {
    if (value.trim())
      parts.push({
        kind: 'text',
        label: 'Prompt',
        text: value.trim(),
        json: false,
        value: undefined,
        truncated: false,
      });
  };
  const fences = fencedRanges(text);
  const opening =
    /(?:^|\n)[ \t]*<(todo|doing|trigger|workspace|permission_request|tool_call|conversation_to_analize|conversation_to_analyze)>/g;
  let position = 0;
  for (const match of text.matchAll(opening)) {
    const start = match.index!;
    if (start < position || fences.some(([from, to]) => start >= from && start < to)) continue;
    if (parts.length >= 32) break;
    const kind = match[1] as keyof typeof referenceNames;
    const bodyStart = start + match[0].length;
    const close = `</${kind}>`;
    const end = text.indexOf(close, bodyStart);
    if (end < 0 && !truncated && !limited) continue;
    addText(text.slice(position, start));
    const body = text.slice(bodyStart, end < 0 ? undefined : end).trim();
    position = end < 0 ? text.length : end + close.length;
    if (event && kind === 'trigger' && body === eventSource.trim()) continue;
    const value = jsonValue(body);
    parts.push({ kind, label: referenceNames[kind], text: body, ...value, truncated: end < 0 });
  }
  addText(text.slice(position));
  return { event, fields: event ? watcherEventFields(event) : [], parts, limited };
}

// Saved input previews can end inside a JSON message. Render only complete rows;
// retain the unchanged preview for original-source/copy/download inspection.
export function watcherConversation(part: WatcherInputPart) {
  const prefix = 'Context excerpt: earlier content omitted.\n';
  const text = part.text.startsWith(prefix) ? part.text.slice(prefix.length) : part.text;
  const parsed = part.json ? { json: true, value: part.value } : jsonValue(text);
  if (
    parsed.json &&
    Array.isArray(parsed.value) &&
    parsed.value.every((message) => typeof watcherObject(message)?.Role === 'string')
  ) {
    return {
      messages: parsed.value.slice(0, 40) as Record<string, unknown>[],
      omitted: parsed.value.length > 40,
      partial: false,
    };
  }
  if (parsed.json || !text.startsWith('[') || !part.truncated) return null;
  const messages: Record<string, unknown>[] = [];
  let index = 1;
  while (index < text.length && messages.length < 40) {
    while (/[\s,]/.test(text[index] ?? '') && index < text.length) index++;
    if (text[index] !== '{') break;
    const start = index;
    let depth = 0,
      quoted = false,
      escaped = false,
      complete = false;
    for (; index < text.length; index++) {
      const character = text[index];
      if (quoted) {
        if (escaped) escaped = false;
        else if (character === '\\') escaped = true;
        else if (character === '"') quoted = false;
      } else if (character === '"') quoted = true;
      else if (character === '{' || character === '[') depth++;
      else if (character === '}' || character === ']') {
        depth--;
        if (depth === 0) {
          index++;
          complete = true;
          break;
        }
      }
    }
    if (!complete) break;
    const message = watcherObject(jsonValue(text.slice(start, index)).value);
    if (!message || typeof message.Role !== 'string') break;
    messages.push(message);
  }
  return { messages, omitted: false, partial: true };
}
