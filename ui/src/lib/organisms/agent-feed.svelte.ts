import { ApiError, HarnessApi } from '../molecules/api/client';
import { subscribeEvents, type StreamState } from '../molecules/api/events';
import {
  eventPayload,
  eventToolCall,
  streamedMessage,
  type AgentPhase,
} from '../atoms/agent-activity';
import { reconcileMessageHistory } from '../atoms/message-history';
import type { HarnessEvent, Message, PermissionRequest, Session, TaskState } from '../atoms/types';

const previewMessageLimit = 80;
const liveTextLimit = 100_000;

// A read-only child feed owns its socket, reads, and bounded live preview. Child
// agents run directly in the loop; their queue status is not an activity signal.
export class AgentFeed {
  session = $state<Session>();
  messages = $state<Message[]>([]);
  liveMessage = $state<Message | null>(null);
  task = $state('');
  taskState = $state<TaskState | null>(null);
  permissions = $state<PermissionRequest[]>([]);
  phase = $state<AgentPhase>('unknown');
  streamState = $state<StreamState>('closed');
  error = $state('');
  loading = $state(false);
  omittedMessages = $state(0);
  liveTruncated = $state(false);
  parentRunning = $state(false);
  parentStartedAt = $state('');
  lastActivityAt = $state('');
  previewEnabled = false;
  private previewReaders = 0;
  private lifetime = new AbortController();
  private closeStream: (() => void) | undefined;
  private refreshTimer: ReturnType<typeof setTimeout> | undefined;
  private pollTimer: ReturnType<typeof setTimeout> | undefined;
  private flushTimer: ReturnType<typeof setTimeout> | undefined;
  private settleTimer: ReturnType<typeof setTimeout> | undefined;
  private savedIDs = new Set<string>();
  private text = '';
  private reasoning = '';
  private refreshing = false;
  private refreshPending = false;

  constructor(
    private api: HarnessApi,
    session: Session,
    private onEvent: (event: HarnessEvent) => void,
    private onSession: (session: Session) => void,
  ) {
    this.session = session;
    if (!session.Completed) this.observe();
  }

  get active() {
    return (
      this.parentRunning &&
      !this.session?.Completed &&
      this.phase === 'working' &&
      this.streamState === 'live' &&
      (!this.parentStartedAt || Date.parse(this.lastActivityAt) >= Date.parse(this.parentStartedAt))
    );
  }

  get label() {
    if (this.phase === 'error') return 'Failed';
    if (this.phase === 'cancelled') return 'Cancelled';
    if (this.phase === 'completed') return 'Finished';
    if (this.session?.Completed || !this.parentRunning) return 'Inactive';
    if (this.streamState === 'retrying') return 'Reconnecting';
    if (this.phase === 'waiting' && this.permissions.length) return 'Waiting for approval';
    if (this.active) return 'Working';
    return this.streamState === 'connecting' ? 'Connecting' : 'Inactive';
  }

  updateSession(session: Session) {
    if (this.lifetime.signal.aborted || session.ID !== this.session?.ID) return;
    const finished = !this.session.Completed && session.Completed;
    this.session = session;
    if (finished) {
      this.scheduleRefresh();
      this.scheduleClose();
    }
  }

  enablePreview() {
    if (this.lifetime.signal.aborted) return () => {};
    this.previewReaders++;
    if (!this.previewEnabled) {
      this.previewEnabled = true;
      this.loading = !this.messages.length;
      void this.refresh();
      this.observe();
    }
    let released = false;
    return () => {
      if (released) return;
      released = true;
      this.previewReaders--;
      if (this.previewReaders > 0) return;
      this.previewEnabled = false;
      clearTimeout(this.refreshTimer);
      this.refreshTimer = undefined;
    };
  }

  private observe() {
    if (this.closeStream || this.lifetime.signal.aborted || !this.session) return;
    this.closeStream = subscribeEvents(
      this.api,
      this.session.ID,
      (event) => this.receiveEvent(event),
      (state) => {
        if (this.lifetime.signal.aborted) return;
        this.streamState = state;
        if (state === 'live' && this.previewEnabled) this.scheduleRefresh();
      },
    );
    this.schedulePoll();
  }

  private schedulePoll() {
    if (
      this.lifetime.signal.aborted ||
      this.pollTimer ||
      ['completed', 'cancelled', 'error'].includes(this.phase)
    )
      return;
    this.pollTimer = setTimeout(async () => {
      this.pollTimer = undefined;
      const session = this.session;
      if (!session || this.lifetime.signal.aborted) return;
      try {
        const updated = await this.api.session(session.ID, this.lifetime.signal);
        if (this.lifetime.signal.aborted) return;
        this.updateSession(updated);
        this.onSession(updated);
        if (this.previewEnabled && this.streamState !== 'live') await this.refresh();
      } catch (failure) {
        this.reportReadFailure(failure);
      }
      if (!this.session?.Completed) this.schedulePoll();
    }, 2500);
  }

  private receiveEvent(event: HarnessEvent) {
    if (this.lifetime.signal.aborted || event.SessionID !== this.session?.ID) return;
    this.onEvent(event);
    const payload = eventPayload(event);
    if (event.Name === 'turn.start' || event.Name === 'model.call') {
      this.phase = 'working';
      this.lastActivityAt = event.Time;
    }
    if (
      event.Name === 'task_state.updated' &&
      typeof payload.Revision === 'number' &&
      payload.SessionID === this.session.ID &&
      Array.isArray(payload.Todo)
    ) {
      this.applyTasks(payload as unknown as TaskState);
    }
    if (
      event.Name === 'permission.request' &&
      typeof payload.ID === 'string' &&
      payload.SessionID === this.session.ID
    ) {
      this.permissions = [
        ...this.permissions.filter((request) => request.ID !== payload.ID),
        payload as unknown as PermissionRequest,
      ];
      this.phase = 'waiting';
    }
    if (event.Name === 'permission.decision') {
      this.permissions = this.permissions.filter((request) => request.ID !== payload.RequestID);
      if (!this.permissions.length && this.phase === 'waiting') this.phase = 'working';
    }
    if (
      ['model.call', 'model.chunk'].includes(event.Name) &&
      typeof payload.message_id === 'string' &&
      !this.savedIDs.has(payload.message_id)
    ) {
      if (this.liveMessage?.ID !== payload.message_id) {
        this.clearLiveMessage();
        this.liveMessage = streamedMessage(event, payload.message_id);
      }
      if (event.Name === 'model.chunk') {
        if (typeof payload.text === 'string') this.text += payload.text;
        if (typeof payload.reasoning === 'string') this.reasoning += payload.reasoning;
        if (this.text.length > liveTextLimit || this.reasoning.length > liveTextLimit)
          this.liveTruncated = true;
        this.text = this.text.slice(-liveTextLimit);
        this.reasoning = this.reasoning.slice(-liveTextLimit);
        if (!this.flushTimer) this.flushTimer = setTimeout(() => this.flushChunks(), 100);
      }
    }
    if (event.Name === 'action.received' && this.liveMessage) {
      const call = eventToolCall(event.Payload);
      if (call && !this.liveMessage.ToolCalls?.some((entry) => entry.ID === call.ID))
        this.liveMessage = {
          ...this.liveMessage,
          ToolCalls: [...(this.liveMessage.ToolCalls ?? []), call],
        };
    }
    if (
      event.Name === 'turn.end' ||
      ['run.error', 'run.cancelled', 'run.interrupted'].includes(event.Name)
    ) {
      this.flushChunks();
      this.permissions = [];
      this.phase =
        payload.status === 'cancelled' || event.Name === 'run.cancelled'
          ? 'cancelled'
          : payload.status === 'error' || ['run.error', 'run.interrupted'].includes(event.Name)
            ? 'error'
            : 'completed';
      this.scheduleRefresh();
      this.scheduleClose();
    } else if (event.Name !== 'model.chunk' && this.previewEnabled) this.scheduleRefresh();
  }

  private flushChunks() {
    clearTimeout(this.flushTimer);
    this.flushTimer = undefined;
    if (!this.liveMessage || this.savedIDs.has(this.liveMessage.ID)) return;
    const text = (this.liveMessage.Content?.[0]?.Text ?? '') + this.text;
    const reasoning = (this.liveMessage.Reasoning ?? '') + this.reasoning;
    if (text.length > liveTextLimit || reasoning.length > liveTextLimit) this.liveTruncated = true;
    this.liveMessage = {
      ...this.liveMessage,
      Reasoning: reasoning.slice(-liveTextLimit),
      Content: text
        ? [
            {
              Type: 'text',
              Text: text.slice(-liveTextLimit),
              Data: null,
              MIME: '',
              URL: '',
              Filename: '',
              AudioID: '',
            },
          ]
        : [],
    };
    this.text = '';
    this.reasoning = '';
  }

  private clearLiveMessage() {
    clearTimeout(this.flushTimer);
    this.flushTimer = undefined;
    this.text = '';
    this.reasoning = '';
    this.liveMessage = null;
    this.liveTruncated = false;
  }

  private scheduleRefresh() {
    if (!this.previewEnabled || this.refreshTimer || this.lifetime.signal.aborted) return;
    this.refreshTimer = setTimeout(() => {
      this.refreshTimer = undefined;
      void this.refresh();
    }, 150);
  }

  async refresh() {
    const session = this.session;
    const signal = this.lifetime.signal;
    if (!session || signal.aborted) return;
    if (this.refreshing) {
      this.refreshPending = true;
      return;
    }
    this.refreshing = true;
    try {
      const [messages, updated, tasks] = await Promise.all([
        this.api.messages(session.ID, signal),
        this.api.session(session.ID, signal),
        this.api.taskState(session.ID, signal).catch(() => null),
      ]);
      if (signal.aborted) return;
      this.savedIDs = new Set(messages.map((message) => message.ID));
      this.task = (messages.find((message) => message.Role === 'user')?.Content ?? [])
        .filter((content) => content.Type === 'text')
        .map((content) => content.Text)
        .join('\n');
      const output = messages.filter(
        (message) => message.Role !== 'user' && message.Role !== 'system',
      );
      this.omittedMessages = Math.max(0, output.length - previewMessageLimit);
      this.messages = reconcileMessageHistory(this.messages, output.slice(-previewMessageLimit));
      if (this.liveMessage && this.savedIDs.has(this.liveMessage.ID)) this.clearLiveMessage();
      if (tasks) this.applyTasks(tasks);
      this.updateSession(updated);
      this.onSession(updated);
      if (updated.Completed) this.scheduleClose();
      this.error = '';
    } catch (failure) {
      this.reportReadFailure(failure);
    } finally {
      if (!signal.aborted) {
        this.loading = false;
        this.refreshing = false;
        if (this.refreshPending) {
          this.refreshPending = false;
          this.scheduleRefresh();
        }
      }
    }
  }

  private applyTasks(state: TaskState) {
    if (
      state.SessionID === this.session?.ID &&
      Number.isSafeInteger(state.Revision) &&
      state.Revision >= (this.taskState?.Revision ?? -1)
    )
      this.taskState = { ...state, Todo: state.Todo ?? [] };
  }

  private reportReadFailure(failure: unknown) {
    if (this.lifetime.signal.aborted) return;
    this.error = failure instanceof Error ? failure.message : 'Cannot load agent output.';
    if (failure instanceof ApiError && failure.status === 404) this.stopObserving();
  }

  async decide(requestID: string, kind: 'allow' | 'deny') {
    await this.api.resolvePermission(requestID, kind);
    if (this.lifetime.signal.aborted) return;
    this.permissions = this.permissions.filter((request) => request.ID !== requestID);
    if (!this.permissions.length && this.phase === 'waiting') this.phase = 'working';
    this.scheduleRefresh();
  }

  private scheduleClose() {
    if (this.settleTimer) return;
    this.settleTimer = setTimeout(() => {
      this.settleTimer = undefined;
      this.stopObserving();
    }, 500);
  }

  private stopObserving() {
    this.closeStream?.();
    this.closeStream = undefined;
    clearTimeout(this.pollTimer);
    this.pollTimer = undefined;
  }

  close() {
    this.lifetime.abort();
    this.stopObserving();
    clearTimeout(this.refreshTimer);
    clearTimeout(this.settleTimer);
    this.clearLiveMessage();
  }
}
