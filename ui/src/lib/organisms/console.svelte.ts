import { ApiError, HarnessApi } from '../molecules/api/client';
import { subscribeEvents, type StreamState } from '../molecules/api/events';
import type {
  HarnessEvent,
  Instance,
  InstanceInput,
  Message,
  Model,
  PermissionRequest,
  QueueStatus,
  Session,
  Statistics,
  TaskState,
} from '../atoms/types';
import { saveConnection, forgetConnection } from '../molecules/connection-storage';

const idleStatus = (): QueueStatus => ({ running: false, queued: 0, messages: [], error: '' });
export class HarnessConsole {
  connection = $state<'disconnected' | 'connecting' | 'connected' | 'error'>('disconnected');
  error = $state('');
  catalogError = $state('');
  instances = $state<Instance[]>([]);
  catalog = $state<Model[]>([]);
  models = $state<Model[]>([]);
  sessions = $state<Session[]>([]);
  instance = $state<Instance | null>(null);
  session = $state<Session | null>(null);
  messages = $state<Message[]>([]);
  liveMessage = $state<Message | null>(null);
  receipts = $state<Message[]>([]);
  permissions = $state<PermissionRequest[]>([]);
  events = $state<HarnessEvent[]>([]);
  status = $state<QueueStatus>(idleStatus());
  statistics = $state<Statistics | null>(null);
  taskState = $state<TaskState | null>(null);
  taskStateError = $state('');
  loading = $state(false);
  streamState = $state<StreamState>('closed');
  private api: HarnessApi | null = null;
  private lifetime = new AbortController();
  private selection = new AbortController();
  private sessionReads = new AbortController();
  private closeStream: (() => void) | undefined;
  private sessionPollTimer: ReturnType<typeof setTimeout> | undefined;
  private refreshTimer: ReturnType<typeof setTimeout> | undefined;
  private refreshing = false;
  private refreshPending = false;
  private savedMessageIDs = new Set<string>();
  private removedSessionIDs = new Set<string>();
  private selectionRevision = 0;
  private streamFlushTimer: ReturnType<typeof setTimeout> | undefined;
  private pendingText: string[] = [];
  private pendingReasoning: string[] = [];

  async connect(base: string, token: string) {
    this.disconnect(false);
    this.connection = 'connecting';
    const signal = this.lifetime.signal;
    try {
      const api = new HarnessApi(base, token);
      this.api = api;
      await api.request('health', 'GET', undefined, signal);
      const instances = await api.instances(signal);
      if (signal.aborted) return;
      this.instances = instances;
      this.connection = 'connected';
      saveConnection({ base, token });
      void this.loadProviderCatalog(api, signal);
      const first = instances.find((instance) => !instance.Stopped) ?? instances[0];
      if (first) await this.selectInstance(first);
    } catch (error) {
      if (signal.aborted) return;
      this.connection = 'error';
      this.error = error instanceof Error ? error.message : 'Connection failed.';
      throw error;
    }
  }

  disconnect(forget = true) {
    this.lifetime.abort();
    this.selection.abort();
    this.stopSessionObservers();
    this.lifetime = new AbortController();
    this.selection = new AbortController();
    this.api = null;
    this.connection = 'disconnected';
    this.error = '';
    this.catalogError = '';
    this.instances = [];
    this.removedSessionIDs.clear();
    this.catalog = [];
    this.models = [];
    this.sessions = [];
    this.instance = null;
    if (forget) forgetConnection();
  }

  private stopSessionObservers() {
    this.sessionReads.abort();
    this.closeStream?.();
    this.closeStream = undefined;
    clearTimeout(this.sessionPollTimer);
    clearTimeout(this.refreshTimer);
    this.clearLiveMessage();
    this.savedMessageIDs.clear();
    this.selectionRevision++;
    this.sessionPollTimer = undefined;
    this.refreshTimer = undefined;
    this.sessionReads = new AbortController();
    this.refreshing = false;
    this.refreshPending = false;
    this.session = null;
    this.messages = [];
    this.receipts = [];
    this.permissions = [];
    this.events = [];
    this.statistics = null;
    this.taskState = null;
    this.taskStateError = '';
    this.status = idleStatus();
    this.streamState = 'closed';
    this.loading = false;
  }

  private requireAPIClient(): HarnessApi {
    if (!this.api || this.connection !== 'connected')
      throw new Error('Connect to the harness first.');
    return this.api;
  }

  private async loadProviderCatalog(api: HarnessApi, signal: AbortSignal) {
    try {
      const providers = await api.providers(signal);
      const results = await Promise.allSettled(
        providers
          .filter((provider) => provider.Connected !== false)
          .map(async (provider) =>
            (await api.providerModels(provider.Name, signal)).map((model) => ({
              ...model,
              ID: `${provider.Name}/${model.ID}`,
            })),
          ),
      );
      if (signal.aborted) return;
      this.catalog = results.flatMap((result) =>
        result.status === 'fulfilled' ? result.value : [],
      );
      this.catalogError = results.some((result) => result.status === 'rejected')
        ? 'Some provider model lists could not be loaded.'
        : '';
    } catch (error) {
      if (!signal.aborted)
        this.catalogError = error instanceof Error ? error.message : 'Cannot load models.';
    }
  }

  async refreshInstances() {
    const api = this.requireAPIClient();
    const signal = this.lifetime.signal;
    const instances = await api.instances(signal);
    if (!signal.aborted) this.instances = instances;
  }

  connectedAPIClient(): HarnessApi {
    return this.requireAPIClient();
  }

  async refreshProviderCatalog() {
    const api = this.requireAPIClient();
    const signal = this.lifetime.signal;
    await this.loadProviderCatalog(api, signal);
    const instance = this.instance;
    if (!signal.aborted && instance && !instance.Stopped) {
      const models = await api.models(instance.ID, signal);
      if (!signal.aborted && this.instance?.ID === instance.ID) this.models = models;
    }
  }

  async selectInstance(instance: Instance) {
    const api = this.requireAPIClient();
    this.selection.abort();
    this.selection = new AbortController();
    this.stopSessionObservers();
    const signal = this.selection.signal;
    this.instance = instance;
    this.sessions = [];
    this.models = [];
    this.error = '';
    this.loading = true;
    try {
      const [sessions, models] = await Promise.all([
        api.sessions(instance.ID, signal),
        instance.Stopped ? Promise.resolve([]) : api.models(instance.ID, signal),
      ]);
      if (signal.aborted) return;
      this.sessions = sessions
        .filter((session) => !this.removedSessionIDs.has(session.ID))
        .sort((first, second) => second.CreatedAt.localeCompare(first.CreatedAt));
      this.models = models;
      const first = this.sessions.find((session) => !session.Parent) ?? this.sessions[0];
      if (first) await this.selectSession(first);
    } catch (error) {
      if (!signal.aborted)
        this.error = error instanceof Error ? error.message : 'Cannot load workspace.';
    } finally {
      if (!signal.aborted) this.loading = false;
    }
  }

  async selectSession(session: Session) {
    if (this.removedSessionIDs.has(session.ID)) return;
    const api = this.requireAPIClient();
    this.stopSessionObservers();
    this.session = session;
    this.error = '';
    this.loading = true;
    const signal = this.sessionReads.signal;
    await this.refreshSession();
    if (signal.aborted) return;
    this.loading = false;
    this.closeStream = subscribeEvents(
      api,
      session.ID,
      (event) => this.receiveEvent(event),
      (state) => {
        if (!signal.aborted) this.streamState = state;
      },
    );
    const pollSessionStatus = async () => {
      if (signal.aborted) return;
      await this.refreshSession();
      if (!signal.aborted) this.sessionPollTimer = setTimeout(pollSessionStatus, 1500);
    };
    this.sessionPollTimer = setTimeout(pollSessionStatus, 1500);
  }

  async refreshSession() {
    const api = this.api;
    const session = this.session;
    const signal = this.sessionReads.signal;
    if (!api || !session || signal.aborted) return;
    if (this.refreshing) {
      this.refreshPending = true;
      return;
    }
    this.refreshing = true;
    const selectionRevision = this.selectionRevision;
    const taskRevision = this.taskState?.Revision ?? -1;
    try {
      const [messages, status, statistics, updatedSession, tasks] = await Promise.all([
        api.messages(session.ID, signal),
        api.status(session.ID, signal),
        api.statistics(session.ID, signal),
        api.session(session.ID, signal),
        api.taskState(session.ID, signal).then(
          (state) => ({ state, error: '' }),
          (error: unknown) => ({
            state: null,
            error: error instanceof Error ? error.message : 'Cannot load task progress.',
          }),
        ),
      ]);
      if (signal.aborted || this.session?.ID !== session.ID) return;
      this.messages = messages;
      this.session = {
        ...updatedSession,
        Model:
          selectionRevision === this.selectionRevision ? updatedSession.Model : this.session.Model,
        ReasoningEffort:
          selectionRevision === this.selectionRevision
            ? updatedSession.ReasoningEffort
            : this.session.ReasoningEffort,
      };
      this.status = { ...status, messages: status.messages ?? [] };
      this.statistics = statistics;
      if (tasks.state) this.applyTaskState(tasks.state);
      else if (taskRevision === (this.taskState?.Revision ?? -1)) this.taskStateError = tasks.error;
      this.error = '';
      const saved = new Set(messages.map((message) => message.ID));
      this.savedMessageIDs = saved;
      if (this.liveMessage && saved.has(this.liveMessage.ID)) this.clearLiveMessage();
      this.receipts = this.receipts.filter(
        (message) =>
          !saved.has(message.ID) && (status.running || status.messages?.includes(message.ID)),
      );
      if (!status.running) this.permissions = [];
    } catch (error) {
      if (
        !signal.aborted &&
        this.session?.ID === session.ID &&
        error instanceof ApiError &&
        error.status === 404
      ) {
        // Another client may delete the conversation while this one is open.
        const removed = new Set([session.ID]);
        for (let changed = true; changed;) {
          changed = false;
          for (const entry of this.sessions) {
            if (entry.Parent && removed.has(entry.Parent) && !removed.has(entry.ID)) {
              removed.add(entry.ID);
              changed = true;
            }
          }
        }
        await this.removeSessionEntries(removed);
        return;
      }
      if (!signal.aborted)
        this.error = error instanceof Error ? error.message : 'Cannot sync the session.';
    } finally {
      if (!signal.aborted) {
        this.refreshing = false;
        if (this.refreshPending) {
          this.refreshPending = false;
          this.scheduleSessionRefresh();
        }
      }
    }
  }

  private scheduleSessionRefresh() {
    if (this.refreshTimer) return;
    this.refreshTimer = setTimeout(() => {
      this.refreshTimer = undefined;
      void this.refreshSession();
    }, 150);
  }

  private receiveEvent(event: HarnessEvent) {
    if (event.Name !== 'model.chunk') this.events = [...this.events.slice(-79), event];
    const payload =
      event.Payload && typeof event.Payload === 'object'
        ? (event.Payload as Record<string, unknown>)
        : {};
    if (
      event.Name === 'task_state.updated' &&
      typeof payload.SessionID === 'string' &&
      typeof payload.Revision === 'number' &&
      Array.isArray(payload.Todo)
    ) {
      this.applyTaskState(payload as unknown as TaskState);
    }
    if (
      (event.Name === 'model.call' || event.Name === 'model.chunk') &&
      typeof payload.message_id === 'string' &&
      !this.savedMessageIDs.has(payload.message_id)
    ) {
      if (this.liveMessage?.ID !== payload.message_id) {
        this.clearLiveMessage();
        this.liveMessage = {
          ID: payload.message_id,
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
      if (event.Name === 'model.chunk') {
        if (typeof payload.text === 'string') this.pendingText.push(payload.text);
        if (typeof payload.reasoning === 'string') this.pendingReasoning.push(payload.reasoning);
        // Batch DOM/Markdown work rather than rerendering on every token.
        if (!this.streamFlushTimer)
          this.streamFlushTimer = setTimeout(() => this.flushModelChunks(), 100);
      }
    }
    if (
      event.Name === 'permission.request' &&
      typeof payload.ID === 'string' &&
      typeof payload.Target === 'string'
    ) {
      const request = payload as unknown as PermissionRequest;
      this.permissions = [...this.permissions.filter((entry) => entry.ID !== request.ID), request];
    }
    if (event.Name === 'permission.decision')
      this.permissions = this.permissions.filter((entry) => entry.ID !== payload.RequestID);
    if (['turn.end', 'run.cancelled', 'run.error', 'run.interrupted'].includes(event.Name))
      this.permissions = [];
    if (['run.cancelled', 'run.error', 'run.interrupted'].includes(event.Name))
      this.clearLiveMessage();
    if (event.Name !== 'model.chunk') this.scheduleSessionRefresh();
  }

  private clearLiveMessage() {
    clearTimeout(this.streamFlushTimer);
    this.streamFlushTimer = undefined;
    this.pendingText = [];
    this.pendingReasoning = [];
    this.liveMessage = null;
  }

  private applyTaskState(state: TaskState) {
    if (state.SessionID !== this.session?.ID || this.removedSessionIDs.has(state.SessionID)) return;
    if (!Number.isSafeInteger(state.Revision) || state.Revision < (this.taskState?.Revision ?? -1))
      return;
    this.taskState = { ...state, Todo: state.Todo ?? [] };
    this.taskStateError = '';
  }

  private flushModelChunks() {
    this.streamFlushTimer = undefined;
    const message = this.liveMessage;
    if (!message || this.savedMessageIDs.has(message.ID)) {
      this.clearLiveMessage();
      return;
    }
    const text = (message.Content?.[0]?.Text ?? '') + this.pendingText.join('');
    this.liveMessage = {
      ...message,
      Reasoning: (message.Reasoning ?? '') + this.pendingReasoning.join(''),
      Content: text
        ? [{ Type: 'text', Text: text, Data: null, MIME: '', URL: '', Filename: '', AudioID: '' }]
        : [],
    };
    this.pendingText = [];
    this.pendingReasoning = [];
  }

  async createInstance(input: InstanceInput) {
    const api = this.requireAPIClient();
    const signal = this.lifetime.signal;
    const instance = await api.createInstance(input);
    if (signal.aborted) return;
    await this.refreshInstances();
    if (!signal.aborted) await this.selectInstance(instance);
  }

  async createSession(model: string, reasoningEffort = '') {
    const api = this.requireAPIClient();
    const instance = this.instance;
    if (!instance) throw new Error('Choose a workspace first.');
    const session = await api.createSession(instance.ID, model, reasoningEffort);
    if (this.api !== api || this.instance?.ID !== instance.ID) return;
    this.sessions = [session, ...this.sessions];
    await this.selectSession(session);
  }

  async setReasoningEffort(effort: string) {
    await this.updateSessionSelection((api, sessionID, signal) =>
      api.setReasoningEffort(sessionID, effort, signal),
    );
  }
  async deleteSession(session: Session): Promise<string[]> {
    const api = this.requireAPIClient();
    const response = await api.deleteSession(session.ID, this.lifetime.signal);
    if (this.api !== api) return [];
    for (const identifier of response.session_ids) this.removedSessionIDs.add(identifier);
    if (this.instance?.ID === session.InstanceID) {
      await this.removeSessionEntries(new Set(response.session_ids));
    }
    return response.session_ids;
  }

  private async removeSessionEntries(removed: Set<string>) {
    for (const identifier of removed) this.removedSessionIDs.add(identifier);
    this.sessions = this.sessions.filter((entry) => !removed.has(entry.ID));
    if (!this.session || !removed.has(this.session.ID)) return;
    this.stopSessionObservers();
    this.error = '';
    const next = this.sessions.find((entry) => !entry.Parent) ?? this.sessions[0];
    if (next) await this.selectSession(next);
  }

  async setSessionModel(model: string, allowCompaction = false) {
    await this.updateSessionSelection((api, sessionID, signal) =>
      api.setSessionModel(sessionID, model, allowCompaction, signal),
    );
  }

  private async updateSessionSelection(
    operation: (api: HarnessApi, sessionID: string, signal: AbortSignal) => Promise<Session>,
  ) {
    const api = this.requireAPIClient();
    const session = this.session;
    const signal = this.sessionReads.signal;
    if (!session) return;
    this.selectionRevision++;
    try {
      const updatedSession = await operation(api, session.ID, signal);
      if (signal.aborted || this.session?.ID !== session.ID) return;
      this.selectionRevision++;
      this.session = updatedSession;
      this.sessions = this.sessions.map((entry) =>
        entry.ID === session.ID ? updatedSession : entry,
      );
    } catch (failure) {
      if (!signal.aborted) throw failure;
    }
  }

  async sendMessage(content: string) {
    const api = this.requireAPIClient();
    const session = this.session;
    if (!session) throw new Error('Create a session first.');
    const receipt = await api.sendMessage(session.ID, content);
    if (this.api === api && this.session?.ID === session.ID) {
      this.receipts = [...this.receipts, receipt.message];
      await this.refreshSession();
    }
    return receipt;
  }

  async cancelCurrentTurn() {
    if (this.session) {
      await this.requireAPIClient().cancelCurrentTurn(this.session.ID);
      await this.refreshSession();
    }
  }
  async cancelQueuedMessage(messageID: string) {
    if (this.session) {
      await this.requireAPIClient().cancelQueuedMessage(this.session.ID, messageID);
      this.receipts = this.receipts.filter((entry) => entry.ID !== messageID);
      await this.refreshSession();
    }
  }
  async resolvePermissionRequest(requestID: string, kind: 'allow' | 'deny') {
    await this.requireAPIClient().resolvePermission(requestID, kind);
    this.permissions = this.permissions.filter((entry) => entry.ID !== requestID);
    this.scheduleSessionRefresh();
  }
  async resumeInstance() {
    const instance = this.instance;
    const api = this.requireAPIClient();
    if (!instance) return;
    const resumed = await api.resumeInstance(instance.ID);
    if (this.api === api) {
      await this.refreshInstances();
      if (this.instance?.ID === instance.ID) await this.selectInstance(resumed);
    }
  }
}
