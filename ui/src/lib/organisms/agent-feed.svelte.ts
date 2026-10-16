import { ApiError, HarnessApi } from '../molecules/api/client';
import { SessionProcesses } from './session-processes.svelte';
import { SessionActivity } from './session-activity.svelte';
import { subscribeEvents, type StreamState } from '../molecules/api/events';
import {
  eventPayload,
  eventToolCall,
  streamedMessage,
  type AgentPhase,
} from '../atoms/agent-activity';
import { SessionHistory } from './session-history.svelte';
import { rememberPermissionDecision, type SessionViewState } from '../atoms/session-view';
import type { HarnessEvent, Message, PermissionRequest, Session, TaskState } from '../atoms/types';

const liveTextLimit = 100_000;

// A read-only child feed owns its socket, reads, and bounded live preview. Child
// agents run directly in the loop; their queue status is not an activity signal.
export class AgentFeed {
  readonly sidekick = new SessionActivity();
  readonly processes = new SessionProcesses();
  readonly history = new SessionHistory((records, messages) => {
    this.processes.applyHistory(
      records,
      messages.map((message) => message.ID),
    );
    this.savedIDs = new Set(messages.map((message) => message.ID));
    if (this.liveMessage && this.savedIDs.has(this.liveMessage.ID)) this.clearLiveMessage();
  });
  session = $state<Session>();
  get messages() {
    return this.history.window.filter(
      (message) => message.Role !== 'user' && message.Role !== 'system',
    );
  }
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
  private serverActive = $state(false);
  private viewVersion = 0;
  private liveRevision = 0;
  private covered = 0;
  private resolvedPermissions = new Set<string>();
  private haveSnapshot = false;

  constructor(
    private api: HarnessApi,
    session: Session,
    private onEvent: (event: HarnessEvent) => void,
    private onSession: (session: Session) => void,
  ) {
    this.session = session;
  }

  get active() {
    return (
      this.serverActive &&
      !this.session?.Completed &&
      this.phase === 'working' &&
      this.streamState === 'live'
    );
  }

  get label() {
    if (this.phase === 'error') return 'Failed';
    if (this.phase === 'cancelled') return 'Cancelled';
    if (this.phase === 'completed') return 'Finished';
    if (this.session?.Completed || !this.serverActive) return 'Inactive';
    if (this.streamState === 'retrying') return 'Reconnecting';
    if (this.phase === 'waiting' && this.permissions.length) return 'Waiting for approval';
    if (this.active) return 'Working';
    return this.streamState === 'connecting' ? 'Connecting' : 'Inactive';
  }
  setActive(active: boolean) {
    if (this.lifetime.signal.aborted) return;
    const changed = this.serverActive !== active;
    this.serverActive = active;
    if (active && !this.session?.Completed) {
      this.observe();
    } else if (changed && !this.previewEnabled) this.stopObserving();
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
      if (this.session) this.processes.open(this.api, this.session.ID);
      if (this.session) this.sidekick.open(this.api, this.session.ID);
      if (this.session) this.history.open(this.api, this.session.ID);
      this.loading = !this.messages.length;
      void this.history.loadLatest().finally(() => {
        if (!this.lifetime.signal.aborted) this.loading = false;
      });
      this.stopObserving();
      this.observe();
    }
    let released = false;
    return () => {
      if (released) return;
      released = true;
      this.previewReaders--;
      if (this.previewReaders > 0) return;
      this.previewEnabled = false;
      this.processes.close();
      this.sidekick.close();
      this.history.close();
      this.savedIDs.clear();
      this.clearLiveMessage();
      clearTimeout(this.refreshTimer);
      this.refreshTimer = undefined;
      this.stopObserving();
      if (this.serverActive) this.observe();
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
        if (state === 'live') {
          clearTimeout(this.pollTimer);
          this.pollTimer = undefined;
        } else if (state !== 'closed') this.schedulePoll();
      },
      {
        details: this.previewEnabled,
        snapshot: (state, initial, covered) => this.receiveState(state, initial, covered),
        recent: (events) => {
          for (const event of events) this.onEvent(event);
        },
        deleted: () => {
          this.error = 'This agent is no longer available.';
          this.close();
        },
      },
    );
    this.schedulePoll();
  }

  private schedulePoll() {
    if (
      this.lifetime.signal.aborted ||
      this.pollTimer ||
      this.streamState === 'live' ||
      (!this.previewEnabled && !this.serverActive)
    )
      return;
    this.pollTimer = setTimeout(async () => {
      this.pollTimer = undefined;
      const session = this.session;
      if (!session || this.lifetime.signal.aborted) return;
      try {
        if (this.streamState !== 'live') await this.refresh();
      } catch (failure) {
        this.reportReadFailure(failure);
      }
      this.schedulePoll();
    }, 5000);
  }

  private receiveState(state: SessionViewState, initial: boolean, covered: number) {
    if (this.lifetime.signal.aborted || state.Session.ID !== this.session?.ID) return;
    const recovery = initial && this.haveSnapshot;
    this.haveSnapshot = true;
    this.viewVersion++;
    this.covered = initial ? covered : Math.max(this.covered, covered);
    this.session = state.Session;
    this.onSession(state.Session);
    this.serverActive = state.Status.running;
    this.permissions = (state.Permissions ?? []).filter(
      (request) => !this.resolvedPermissions.has(request.ID),
    );
    this.phase = this.permissions.length
      ? 'waiting'
      : this.serverActive
        ? 'working'
        : state.Outcome === 'cancelled'
          ? 'cancelled'
          : state.Status.error || state.Outcome === 'error'
            ? 'error'
            : state.Session.Completed
              ? 'completed'
              : 'unknown';
    this.task = state.History.Task ?? this.task;
    if (state.TaskState) this.applyTasks(state.TaskState);
    if (this.previewEnabled) {
      const previousHead = this.history.headSeq;
      this.history.observeMetadata(state.History);
      if (recovery) void this.history.recover();
      this.processes.applyRecords(state.Processes ?? []);
      if (
        initial ||
        previousHead !== state.History.HeadSeq ||
        state.Live?.Message.ID !== this.liveMessage?.ID
      )
        this.sidekick.scheduleRefresh();
      if (
        state.Live &&
        state.Live.Message.ID !== this.history.latest?.MessageID &&
        !this.savedIDs.has(state.Live.Message.ID)
      ) {
        this.clearLiveMessage();
        this.liveMessage = state.Live.Message;
        this.liveRevision = state.Live.Revision;
        this.liveTruncated = state.Live.Truncated;
      } else if (!this.serverActive || this.liveMessage?.ID === this.history.latest?.MessageID)
        this.clearLiveMessage();
    }
    this.error = '';
    if (!this.serverActive) this.scheduleClose();
  }

  private receiveEvent(event: HarnessEvent) {
    if (this.lifetime.signal.aborted || event.SessionID !== this.session?.ID) return;
    if (event.Seq <= this.covered) {
      if (event.Name === 'agent.start' || event.Name === 'agent.end') this.onEvent(event);
      return;
    }
    this.onEvent(event);
    this.processes.observe(event);
    this.sidekick.observe(event);
    if (['process.start', 'process.output', 'process.exit'].includes(event.Name)) return;
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
      !this.resolvedPermissions.has(payload.ID) &&
      payload.SessionID === this.session.ID
    ) {
      this.permissions = [
        ...this.permissions.filter((request) => request.ID !== payload.ID),
        payload as unknown as PermissionRequest,
      ];
      this.phase = 'waiting';
    }
    if (event.Name === 'permission.decision') {
      if (typeof payload.RequestID === 'string')
        rememberPermissionDecision(this.resolvedPermissions, payload.RequestID);
      this.permissions = this.permissions.filter((request) => request.ID !== payload.RequestID);
      if (!this.permissions.length && this.phase === 'waiting') this.phase = 'working';
    }
    if (
      this.previewEnabled &&
      ['model.call', 'model.chunk'].includes(event.Name) &&
      typeof payload.message_id === 'string' &&
      !this.savedIDs.has(payload.message_id)
    ) {
      if (this.liveMessage?.ID !== payload.message_id) {
        this.clearLiveMessage();
        this.liveMessage = streamedMessage(event, payload.message_id);
      }
      if (event.Name === 'model.chunk') {
        if (typeof payload.chunk_index === 'number' && payload.chunk_index > 0) {
          if (payload.chunk_index <= this.liveRevision) return;
          this.liveRevision = payload.chunk_index;
        }
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
          ToolCalls: [...(this.liveMessage.ToolCalls ?? []), call].slice(-32),
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
      if (this.streamState !== 'live') this.scheduleRefresh();
      if (!this.previewEnabled) this.scheduleClose();
    } else if (event.Name !== 'model.chunk' && this.previewEnabled && this.streamState !== 'live')
      this.scheduleRefresh();
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
    this.liveRevision = 0;
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
    if (this.streamState === 'live') {
      if (this.previewEnabled) await this.history.refresh();
      return;
    }
    if (this.refreshing) {
      this.refreshPending = true;
      return;
    }
    this.refreshing = true;
    const version = this.viewVersion;
    try {
      const state = await this.api.sessionView(session.ID, this.previewEnabled, signal);
      if (signal.aborted) return;
      if (version === this.viewVersion) this.receiveState(state, false, this.covered);
      if (this.previewEnabled) await this.history.refresh();
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
    rememberPermissionDecision(this.resolvedPermissions, requestID);
    this.permissions = this.permissions.filter((request) => request.ID !== requestID);
    if (!this.permissions.length && this.phase === 'waiting') this.phase = 'working';
    this.scheduleRefresh();
  }

  private scheduleClose() {
    if (this.settleTimer) return;
    this.settleTimer = setTimeout(() => {
      this.settleTimer = undefined;
      if (
        !this.serverActive &&
        (!this.previewEnabled ||
          this.session?.Completed ||
          ['completed', 'cancelled', 'error'].includes(this.phase))
      )
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
    this.processes.close();
    this.sidekick.close();
    this.history.close();
    this.savedIDs.clear();
    this.lifetime.abort();
    this.stopObserving();
    clearTimeout(this.refreshTimer);
    clearTimeout(this.settleTimer);
    this.clearLiveMessage();
  }
}
