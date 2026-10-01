import { HarnessApi } from '../molecules/api/client';
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
  receipts = $state<Message[]>([]);
  permissions = $state<PermissionRequest[]>([]);
  events = $state<HarnessEvent[]>([]);
  status = $state<QueueStatus>(idleStatus());
  statistics = $state<Statistics | null>(null);
  loading = $state(false);
  streamState = $state<StreamState>('closed');
  private api: HarnessApi | null = null;
  private lifetime = new AbortController();
  private selection = new AbortController();
  private sessionReads = new AbortController();
  private closeStream: (() => void) | undefined;
  private poll: ReturnType<typeof setTimeout> | undefined;
  private refreshTimer: ReturnType<typeof setTimeout> | undefined;
  private refreshing = false;
  private refreshPending = false;

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
      void this.loadCatalog(api, signal);
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
    this.stopSession();
    this.lifetime = new AbortController();
    this.selection = new AbortController();
    this.api = null;
    this.connection = 'disconnected';
    this.error = '';
    this.catalogError = '';
    this.instances = [];
    this.catalog = [];
    this.models = [];
    this.sessions = [];
    this.instance = null;
    if (forget) forgetConnection();
  }

  private stopSession() {
    this.sessionReads.abort();
    this.closeStream?.();
    this.closeStream = undefined;
    clearTimeout(this.poll);
    clearTimeout(this.refreshTimer);
    this.poll = undefined;
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
    this.status = idleStatus();
    this.streamState = 'closed';
    this.loading = false;
  }

  private client(): HarnessApi {
    if (!this.api || this.connection !== 'connected')
      throw new Error('Connect to the harness first.');
    return this.api;
  }

  private async loadCatalog(api: HarnessApi, signal: AbortSignal) {
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
    const api = this.client();
    const signal = this.lifetime.signal;
    const instances = await api.instances(signal);
    if (!signal.aborted) this.instances = instances;
  }

  providerClient(): HarnessApi {
    return this.client();
  }

  async refreshProviderCatalog() {
    const api = this.client();
    const signal = this.lifetime.signal;
    await this.loadCatalog(api, signal);
    const instance = this.instance;
    if (!signal.aborted && instance && !instance.Stopped) {
      const models = await api.models(instance.ID, signal);
      if (!signal.aborted && this.instance?.ID === instance.ID) this.models = models;
    }
  }

  async selectInstance(instance: Instance) {
    const api = this.client();
    this.selection.abort();
    this.selection = new AbortController();
    this.stopSession();
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
      this.sessions = sessions.sort((first, second) =>
        second.CreatedAt.localeCompare(first.CreatedAt),
      );
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
    const api = this.client();
    this.stopSession();
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
    const poll = async () => {
      if (signal.aborted) return;
      await this.refreshSession();
      if (!signal.aborted) this.poll = setTimeout(poll, 1500);
    };
    this.poll = setTimeout(poll, 1500);
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
    try {
      const [messages, status, statistics] = await Promise.all([
        api.messages(session.ID, signal),
        api.status(session.ID, signal),
        api.statistics(session.ID, signal),
      ]);
      if (signal.aborted || this.session?.ID !== session.ID) return;
      this.messages = messages;
      this.status = { ...status, messages: status.messages ?? [] };
      this.statistics = statistics;
      this.error = '';
      const saved = new Set(messages.map((message) => message.ID));
      this.receipts = this.receipts.filter(
        (message) =>
          !saved.has(message.ID) && (status.running || status.messages?.includes(message.ID)),
      );
      if (!status.running) this.permissions = [];
    } catch (error) {
      if (!signal.aborted)
        this.error = error instanceof Error ? error.message : 'Cannot sync the session.';
    } finally {
      if (!signal.aborted) {
        this.refreshing = false;
        if (this.refreshPending) {
          this.refreshPending = false;
          this.scheduleRefresh();
        }
      }
    }
  }

  private scheduleRefresh() {
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
    if (event.Name !== 'model.chunk') this.scheduleRefresh();
  }

  async createInstance(input: InstanceInput) {
    const api = this.client();
    const signal = this.lifetime.signal;
    const instance = await api.createInstance(input);
    if (signal.aborted) return;
    await this.refreshInstances();
    if (!signal.aborted) await this.selectInstance(instance);
  }

  async createSession(model: string) {
    const api = this.client();
    const instance = this.instance;
    if (!instance) throw new Error('Choose a workspace first.');
    const session = await api.createSession(instance.ID, model);
    if (this.api !== api || this.instance?.ID !== instance.ID) return;
    this.sessions = [session, ...this.sessions];
    await this.selectSession(session);
  }

  async send(content: string) {
    const api = this.client();
    const session = this.session;
    if (!session) throw new Error('Create a session first.');
    const receipt = await api.send(session.ID, content);
    if (this.api === api && this.session?.ID === session.ID) {
      this.receipts = [...this.receipts, receipt.message];
      await this.refreshSession();
    }
    return receipt;
  }

  async cancel() {
    if (this.session) {
      await this.client().cancel(this.session.ID);
      await this.refreshSession();
    }
  }
  async cancelQueued(messageID: string) {
    if (this.session) {
      await this.client().cancelQueued(this.session.ID, messageID);
      this.receipts = this.receipts.filter((entry) => entry.ID !== messageID);
      await this.refreshSession();
    }
  }
  async decide(requestID: string, kind: 'allow' | 'deny') {
    await this.client().resolvePermission(requestID, kind);
    this.permissions = this.permissions.filter((entry) => entry.ID !== requestID);
    this.scheduleRefresh();
  }
  async resumeInstance() {
    const instance = this.instance;
    const api = this.client();
    if (!instance) return;
    const resumed = await api.resume(instance.ID);
    if (this.api === api) {
      await this.refreshInstances();
      if (this.instance?.ID === instance.ID) await this.selectInstance(resumed);
    }
  }
}
