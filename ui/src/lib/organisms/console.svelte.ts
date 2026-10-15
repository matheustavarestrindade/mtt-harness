import { SvelteMap } from 'svelte/reactivity';
import { ApiError, HarnessApi } from '../molecules/api/client';
import { subscribeEvents, type StreamState } from '../molecules/api/events';
import type {
  Content,
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
import { reconcileMessageHistory } from '../atoms/message-history';
import { latestRequestUsage } from '../atoms/request-usage';
import { eventToolCall } from '../atoms/agent-activity';
import { sessionRoot } from '../atoms/session-tree';
import { AgentMonitor } from './agent-monitor.svelte';
import { SessionProcesses } from './session-processes.svelte';
import { SessionActivity } from './session-activity.svelte';
import {
  readNavigationRoute,
  writeNavigationRoute,
  observeNavigationRoute,
} from '../molecules/browser-navigation';
import type {
  NavigationHistory,
  NavigationRoute,
  WorkspaceNavigation,
} from '../atoms/navigation-route';

const idleStatus = (): QueueStatus => ({ running: false, queued: 0, messages: [], error: '' });
export class HarnessConsole {
  readonly sidekick = new SessionActivity();
  readonly processes = new SessionProcesses();
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
  readonly agents = new AgentMonitor(
    () => this.scheduleSessionListRefresh(),
    (session) => this.updateAgentSession(session),
    (agent, history) => {
      if (this.instance && this.session)
        writeNavigationRoute(
          { workspace: this.instance.ID, session: this.session.ID, agent },
          history,
        );
    },
  );
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
  private requestModels = new SvelteMap<string, string>();
  private removedSessionIDs = new Set<string>();
  private selectionRevision = 0;
  private streamFlushTimer: ReturnType<typeof setTimeout> | undefined;
  private pendingText: string[] = [];
  private pendingReasoning: string[] = [];
  private sessionListTimer: ReturnType<typeof setTimeout> | undefined;
  private refreshingSessionList = false;
  private sessionListPending = false;
  private navigationRevision = 0;

  observeNavigation() {
    return observeNavigationRoute((route) => {
      if (this.connection === 'connected') void this.restoreNavigation(route);
    });
  }

  private async restoreNavigation(route: NavigationRoute) {
    const revision = ++this.navigationRevision;
    const api = this.api,
      signal = this.lifetime.signal;
    if (!api || signal.aborted) return;
    let workspaceID = route.workspace;
    try {
      if (!workspaceID && route.session)
        workspaceID = (await api.session(route.session, signal)).InstanceID;
      if (signal.aborted || revision !== this.navigationRevision) return;
      const instance = workspaceID
        ? this.instances.find((entry) => entry.ID === workspaceID)
        : (this.instances.find((entry) => !entry.Stopped) ?? this.instances[0]);
      if (!instance) {
        this.selection.abort();
        this.stopSessionObservers();
        this.instance = null;
        this.sessions = [];
        this.models = [];
        this.error = workspaceID ? 'The workspace in this URL is not available.' : '';
        writeNavigationRoute({ workspace: '', session: '', agent: '' }, 'replace');
        return;
      }
      if (this.instance?.ID !== instance.ID || this.loading || !this.sessions.length) {
        await this.selectInstance(instance, {
          session: route.session || undefined,
          agent: route.agent,
          history: 'replace',
        });
        return;
      }
      const target = route.session
        ? this.sessions.find((entry) => entry.ID === route.session)
        : this.sessions.find((entry) => !entry.Parent);
      if (!target) {
        this.stopSessionObservers();
        this.error = route.session
          ? 'The session in this URL is not available in this workspace.'
          : '';
        writeNavigationRoute({ workspace: instance.ID, session: '', agent: '' }, 'replace');
        return;
      }
      await this.selectNavigationTarget(target, route.agent || '', 'replace');
    } catch (failure) {
      if (!signal.aborted && revision === this.navigationRevision)
        this.error = failure instanceof Error ? failure.message : 'Cannot restore this page.';
    }
  }

  private async selectNavigationTarget(
    target: Session,
    agentID: string,
    history: NavigationHistory,
  ) {
    const root = sessionRoot(target, this.sessions);
    if (!root) return;
    const agent = agentID
      ? this.sessions.find(
          (entry) =>
            entry.ID === agentID &&
            sessionRoot(entry, this.sessions)?.ID === root.ID &&
            entry.Parent,
        )
      : target.Parent
        ? target
        : undefined;
    writeNavigationRoute(
      { workspace: root.InstanceID, session: root.ID, agent: agent?.ID ?? '' },
      history,
    );
    const selected = this.selectSession(root, false);
    const revision = this.navigationRevision;
    await selected;
    if (revision !== this.navigationRevision || this.session?.ID !== root.ID) return;
    if (agent) this.agents.focus(agent.ID);
    else {
      this.agents.clearFocus('replace');
      if (agentID) this.error = 'The agent in this URL is not available in this conversation.';
    }
  }

  get requestUsage() {
    return latestRequestUsage(this.messages, this.session?.ID ?? '', this.requestModels, [
      ...this.models,
      ...this.catalog,
    ]);
  }

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
      await this.restoreNavigation(readNavigationRoute());
    } catch (error) {
      if (signal.aborted) return;
      this.connection = 'error';
      this.error = error instanceof Error ? error.message : 'Connection failed.';
      throw error;
    }
  }

  disconnect(forget = true) {
    this.navigationRevision++;
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
    this.sidekick.close();
    this.processes.close();
    this.agents.close();
    clearTimeout(this.sessionListTimer);
    this.sessionListTimer = undefined;
    this.refreshingSessionList = false;
    this.sessionListPending = false;
    this.sessionReads.abort();
    this.closeStream?.();
    this.closeStream = undefined;
    clearTimeout(this.sessionPollTimer);
    clearTimeout(this.refreshTimer);
    this.clearLiveMessage();
    this.savedMessageIDs.clear();
    this.requestModels.clear();
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

  async selectInstance(instance: Instance, navigation: WorkspaceNavigation = {}) {
    const revision = ++this.navigationRevision;
    const history = navigation.history === undefined ? 'push' : navigation.history;
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
    writeNavigationRoute(
      { workspace: instance.ID, session: navigation.session ?? '', agent: navigation.agent ?? '' },
      history,
    );
    try {
      const [sessions, models] = await Promise.all([
        api.sessions(instance.ID, signal),
        instance.Stopped ? Promise.resolve([]) : api.models(instance.ID, signal),
      ]);
      if (signal.aborted || revision !== this.navigationRevision) return;
      this.sessions = sessions
        .filter((session) => !this.removedSessionIDs.has(session.ID))
        .sort((first, second) => second.CreatedAt.localeCompare(first.CreatedAt));
      this.models = models;
      const target = navigation.session
        ? this.sessions.find((session) => session.ID === navigation.session)
        : this.sessions.find((session) => !session.Parent);
      if (target) await this.selectNavigationTarget(target, navigation.agent ?? '', 'replace');
      else {
        writeNavigationRoute({ workspace: instance.ID, session: '', agent: '' }, 'replace');
        if (navigation.session)
          this.error = 'The session in this URL is not available in this workspace.';
      }
    } catch (error) {
      if (!signal.aborted && revision === this.navigationRevision)
        this.error = error instanceof Error ? error.message : 'Cannot load workspace.';
    } finally {
      if (!signal.aborted && revision === this.navigationRevision) this.loading = false;
    }
  }

  async selectSession(session: Session, history: NavigationHistory = 'push') {
    if (this.removedSessionIDs.has(session.ID)) return;
    const api = this.requireAPIClient();
    this.navigationRevision++;
    if (this.session?.ID === session.ID) {
      if (history) this.agents.clearFocus(history);
      writeNavigationRoute(
        { workspace: session.InstanceID, session: session.ID, agent: '' },
        history,
      );
      return;
    }
    this.stopSessionObservers();
    this.session = session;
    writeNavigationRoute(
      { workspace: session.InstanceID, session: session.ID, agent: '' },
      history,
    );
    this.processes.open(api, session.ID);
    this.sidekick.open(api, session.ID);
    this.agents.open(api, session, this.sessions);
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
        if (signal.aborted) return;
        const previous = this.streamState;
        this.streamState = state;
        if (state === 'live' && previous !== 'live') {
          this.scheduleSessionRefresh();
          this.processes.scheduleRefresh();
          this.sidekick.scheduleRefresh();
        }
      },
    );
    const pollSessionStatus = async () => {
      if (signal.aborted) return;
      await this.pollSessionMetadata();
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
      this.messages = reconcileMessageHistory(this.messages, messages);
      this.sidekick.scheduleRefresh();
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
      this.agents.sync(this.sessions, status.running);
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

  private scheduleSessionListRefresh() {
    if (this.sessionListTimer || this.sessionReads.signal.aborted) return;
    this.sessionListTimer = setTimeout(() => {
      this.sessionListTimer = undefined;
      void this.refreshSessionList();
    }, 150);
  }

  private async refreshSessionList() {
    const api = this.api;
    const instance = this.instance;
    const signal = this.sessionReads.signal;
    if (!api || !instance || signal.aborted) return;
    if (this.refreshingSessionList) {
      this.sessionListPending = true;
      return;
    }
    this.refreshingSessionList = true;
    try {
      const sessions = await api.sessions(instance.ID, signal);
      if (signal.aborted || this.instance?.ID !== instance.ID) return;
      this.sessions = sessions
        .filter((session) => !this.removedSessionIDs.has(session.ID))
        .map((session) => (session.ID === this.session?.ID ? this.session : session))
        .sort((first, second) => second.CreatedAt.localeCompare(first.CreatedAt));
      this.agents.sync(this.sessions, this.status.running);
      this.agents.error = '';
    } catch (failure) {
      // Parent synchronization reports connection failures. Keep existing agent
      // views until an authoritative session list confirms their removal.
      if (!signal.aborted)
        this.agents.error = failure instanceof Error ? failure.message : 'Cannot load agents.';
    } finally {
      if (!signal.aborted) {
        this.refreshingSessionList = false;
        if (this.sessionListPending) {
          this.sessionListPending = false;
          this.scheduleSessionListRefresh();
        }
      }
    }
  }

  private updateAgentSession(session: Session) {
    if (this.removedSessionIDs.has(session.ID) || session.InstanceID !== this.instance?.ID) return;
    this.sessions = this.sessions.map((entry) => (entry.ID === session.ID ? session : entry));
  }

  async selectAgent(session: Session) {
    const root = sessionRoot(session, this.sessions);
    if (!root || this.removedSessionIDs.has(session.ID)) return null;
    writeNavigationRoute({ workspace: root.InstanceID, session: root.ID, agent: session.ID });
    const selecting = this.selectSession(root, false);
    const revision = this.navigationRevision;
    await selecting;
    if (revision !== this.navigationRevision || this.session?.ID !== root.ID) return null;
    return this.agents.focus(session.ID);
  }

  private async pollSessionMetadata() {
    const api = this.api;
    const session = this.session;
    const signal = this.sessionReads.signal;
    if (!api || !session || signal.aborted) return;
    // Events drive history synchronization. A disconnected stream falls back to
    // full reads; a healthy idle stream needs only small lifecycle metadata.
    if (this.streamState !== 'live') {
      await this.refreshSession();
      return;
    }
    const selectionRevision = this.selectionRevision;
    try {
      const [status, updatedSession] = await Promise.all([
        api.status(session.ID, signal),
        api.session(session.ID, signal),
      ]);
      if (signal.aborted || this.session?.ID !== session.ID) return;
      const queued = status.messages ?? [];
      const previousQueued = this.status.messages ?? [];
      const changed =
        status.running !== this.status.running ||
        status.queued !== this.status.queued ||
        status.error !== this.status.error ||
        queued.length !== previousQueued.length ||
        queued.some((identifier, index) => identifier !== previousQueued[index]) ||
        updatedSession.Completed !== this.session.Completed;
      this.session = {
        ...updatedSession,
        Model:
          selectionRevision === this.selectionRevision ? updatedSession.Model : this.session.Model,
        ReasoningEffort:
          selectionRevision === this.selectionRevision
            ? updatedSession.ReasoningEffort
            : this.session.ReasoningEffort,
      };
      if (changed) await this.refreshSession();
      if (status.running) this.scheduleSessionListRefresh();
    } catch {
      // The authoritative refresh handles deletion, cancellation and API errors.
      if (!signal.aborted) await this.refreshSession();
    }
  }

  private receiveEvent(event: HarnessEvent) {
    if (event.SessionID !== this.session?.ID) return;
    this.sidekick.observe(event);
    this.processes.observe(event);
    if (['process.start', 'process.output', 'process.exit'].includes(event.Name)) return;
    this.agents.observe(event);
    if (event.Name !== 'model.chunk') this.events = [...this.events.slice(-79), event];
    const payload =
      event.Payload && typeof event.Payload === 'object'
        ? (event.Payload as Record<string, unknown>)
        : {};
    if (
      event.Name === 'model.call' &&
      typeof payload.message_id === 'string' &&
      typeof payload.model === 'string'
    ) {
      this.requestModels.set(payload.message_id, payload.model);
    }
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
    if (event.Name === 'action.received' && this.liveMessage) {
      const call = eventToolCall(event.Payload);
      if (call && !this.liveMessage.ToolCalls?.some((entry) => entry.ID === call.ID))
        this.liveMessage = {
          ...this.liveMessage,
          ToolCalls: [...(this.liveMessage.ToolCalls ?? []), call],
        };
    }
    if (event.Name === 'permission.decision')
      this.permissions = this.permissions.filter((entry) => entry.ID !== payload.RequestID);
    if (['turn.end', 'run.cancelled', 'run.error', 'run.interrupted'].includes(event.Name))
      this.permissions = [];
    if (['turn.end', 'run.cancelled', 'run.error', 'run.interrupted'].includes(event.Name))
      this.scheduleSessionListRefresh();
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
    this.agents.sync(this.sessions, this.status.running);
    if (!this.session || !removed.has(this.session.ID)) return;
    this.stopSessionObservers();
    this.error = '';
    const next = this.sessions.find((entry) => !entry.Parent);
    if (next) await this.selectSession(next, 'replace');
    else
      writeNavigationRoute(
        { workspace: this.instance?.ID ?? '', session: '', agent: '' },
        'replace',
      );
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

  async sendMessage(content: string | Content[]) {
    const api = this.requireAPIClient();
    const session = this.session;
    if (!session) throw new Error('Create a session first.');
    const receipt = await api.sendMessage(session.ID, content);
    if (this.api === api && this.session?.ID === session.ID) {
      this.receipts = [...this.receipts, receipt.message];
      // Acceptance is authoritative even if the subsequent status read fails.
      // Keeping a sent attachment as a retryable draft would duplicate the turn.
      try {
        await this.refreshSession();
      } catch (error) {
        if (this.api === api && this.session?.ID === session.ID)
          this.error =
            error instanceof Error ? error.message : 'Cannot refresh the accepted message.';
      }
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
      if (this.instance?.ID === instance.ID)
        await this.selectInstance(resumed, {
          session: this.session?.ID,
          agent: this.agents.focusedID,
          history: 'replace',
        });
    }
  }
}
