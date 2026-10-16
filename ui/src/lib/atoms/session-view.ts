import type {
  HarnessEvent,
  Message,
  PermissionRequest,
  QueueStatus,
  Session,
  Statistics,
  TaskState,
  Usage,
} from './types';
import type { ProcessRecord } from './processes';
import type { QueuedMessagePreview } from './queued-messages';

export interface ResponseUsageSnapshot {
  MessageID: string;
  Seq: number;
  Model: string;
  Usage: Usage | null;
  CreatedAt: string;
}
export interface HistoryPage {
  SessionID: string;
  Revision: number;
  Messages: Message[];
  Context: Message[];
  Models: Record<string, string>;
  Sizes: Record<string, number>;
  FirstSeq: number;
  LastSeq: number;
  HeadSeq: number;
  HasOlder: boolean;
  HasNewer: boolean;
  LatestResponse: ResponseUsageSnapshot | null;
  Processes: ProcessRecord[];
  Task?: string;
}
export interface HistoryQuery {
  before?: number;
  after?: number;
  limit?: number;
  revision?: number;
  metadata?: boolean;
}
export interface SessionViewState {
  Session: Session;
  Status: QueueStatus;
  Statistics: Statistics;
  TaskState: TaskState | null;
  TaskStateError?: string;
  Outcome?: string;
  Permissions: PermissionRequest[];
  History: HistoryPage;
  Live: { Message: Message; Model: string; Revision: number; Truncated: boolean } | null;
  Processes: ProcessRecord[];
  WorkspaceLinked: boolean;
  WorkspaceStopped: boolean;
  ActiveSessions: string[];
  QueuedMessages?: QueuedMessagePreview[];
}
export interface SessionViewFrame {
  type: 'state';
  session_id: string;
  cursor: number;
  covered_cursor?: number;
  state: SessionViewState;
  initial: boolean;
  recent?: HarnessEvent[];
}

export function historyUsageMessage(snapshot: ResponseUsageSnapshot, sessionID: string): Message {
  return {
    ID: snapshot.MessageID,
    SessionID: sessionID,
    Seq: snapshot.Seq,
    Role: 'assistant',
    Content: [],
    ToolCalls: [],
    ToolCallID: '',
    Usage: snapshot.Usage,
    CreatedAt: snapshot.CreatedAt,
  };
}

// A permission ID has one decision. Bound acknowledgements so an older socket
// snapshot cannot restore a card after the HTTP decision has succeeded.
export function rememberPermissionDecision(identifiers: Set<string>, identifier: string) {
  identifiers.add(identifier);
  while (identifiers.size > 256) {
    const oldest = identifiers.values().next().value;
    if (!oldest) break;
    identifiers.delete(oldest);
  }
}
