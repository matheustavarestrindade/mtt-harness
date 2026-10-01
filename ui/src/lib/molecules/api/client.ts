import type {
  AcceptedMessage,
  HarnessEvent,
  Instance,
  InstanceInput,
  Message,
  Model,
  Provider,
  DeviceLogin,
  QueueStatus,
  Session,
  Statistics,
} from '../../atoms/types';

export class ApiError extends Error {
  constructor(
    message: string,
    public readonly status: number,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

export class HarnessApi {
  readonly base: URL;
  constructor(
    base: string,
    private readonly token: string,
  ) {
    this.base = new URL(`${base.trim().replace(/\/+$/, '')}/`, window.location.origin);
    if (
      !['http:', 'https:'].includes(this.base.protocol) ||
      this.base.search ||
      this.base.hash ||
      this.base.username ||
      this.base.password
    ) {
      throw new Error('Use an HTTP API address or a path such as /api.');
    }
  }

  private url(path: string): URL {
    return new URL(path.replace(/^\//, ''), this.base);
  }

  async request<Value>(
    path: string,
    method = 'GET',
    body?: unknown,
    signal?: AbortSignal,
    timeoutMilliseconds = 20_000,
  ): Promise<Value> {
    const timeout = AbortSignal.timeout(timeoutMilliseconds);
    const requestSignal = signal ? AbortSignal.any([signal, timeout]) : timeout;
    let response: Response;
    try {
      response = await fetch(this.url(path), {
        method,
        signal: requestSignal,
        cache: 'no-store',
        headers: {
          Accept: 'application/json',
          ...(this.token ? { Authorization: `Bearer ${this.token}` } : {}),
          ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
        },
        body: body === undefined ? undefined : JSON.stringify(body),
      });
    } catch (error) {
      if (signal?.aborted) throw error;
      if (timeout.aborted)
        throw new Error('The API request timed out. Check the connection and try again.');
      throw new Error(
        'Cannot reach the API. Use /api with the UI proxy and check HARNESS_API_URL in ui/.env.',
      );
    }
    const text = await response.text();
    let data: unknown;
    try {
      data = text ? JSON.parse(text) : null;
    } catch {
      throw new ApiError(
        `The API returned a non-JSON response (${response.status}). Check the proxy target.`,
        response.status,
      );
    }
    if (!response.ok) {
      const detail =
        data && typeof data === 'object' && 'error' in data
          ? String(data.error)
          : `Request failed (${response.status}).`;
      throw new ApiError(detail, response.status);
    }
    return data as Value;
  }

  async instances(signal?: AbortSignal) {
    return (await this.request<Instance[] | null>('instances', 'GET', undefined, signal)) ?? [];
  }
  async sessions(instanceID: string, signal?: AbortSignal) {
    return (
      (await this.request<Session[] | null>(
        `instances/${encodeURIComponent(instanceID)}/sessions`,
        'GET',
        undefined,
        signal,
      )) ?? []
    );
  }
  async models(instanceID: string, signal?: AbortSignal) {
    return (
      (await this.request<Model[] | null>(
        `instances/${encodeURIComponent(instanceID)}/models`,
        'GET',
        undefined,
        signal,
      )) ?? []
    );
  }
  async providers(signal?: AbortSignal) {
    return (await this.request<Provider[] | null>('providers', 'GET', undefined, signal)) ?? [];
  }
  saveProviderKey(providerID: string, key: string, signal?: AbortSignal) {
    return this.request(`providers/${encodeURIComponent(providerID)}/key`, 'PUT', { key }, signal);
  }
  deleteProviderKey(providerID: string, signal?: AbortSignal) {
    return this.request(
      `providers/${encodeURIComponent(providerID)}/key`,
      'DELETE',
      undefined,
      signal,
    );
  }
  refreshProvider(providerID: string, signal?: AbortSignal) {
    return this.request<Model[]>(
      `providers/${encodeURIComponent(providerID)}/refresh`,
      'POST',
      undefined,
      signal,
      45_000,
    );
  }
  startProviderLogin(providerID: string, signal?: AbortSignal) {
    return this.request<DeviceLogin>(
      `providers/${encodeURIComponent(providerID)}/auth/device`,
      'POST',
      undefined,
      signal,
      30_000,
    );
  }
  providerLogin(providerID: string, loginID: string, signal?: AbortSignal) {
    return this.request<DeviceLogin>(
      `providers/${encodeURIComponent(providerID)}/auth/device/${encodeURIComponent(loginID)}`,
      'GET',
      undefined,
      signal,
    );
  }
  cancelProviderLogin(providerID: string, loginID: string, signal?: AbortSignal) {
    return this.request(
      `providers/${encodeURIComponent(providerID)}/auth/device/${encodeURIComponent(loginID)}`,
      'DELETE',
      undefined,
      signal,
    );
  }
  disconnectCodingPlan(providerID: string, signal?: AbortSignal) {
    return this.request(
      `providers/${encodeURIComponent(providerID)}/auth`,
      'DELETE',
      undefined,
      signal,
    );
  }
  async providerModels(providerID: string, signal?: AbortSignal) {
    return (
      (await this.request<Model[] | null>(
        `providers/${encodeURIComponent(providerID)}/models`,
        'GET',
        undefined,
        signal,
      )) ?? []
    );
  }
  createInstance(input: InstanceInput) {
    return this.request<Instance>('instances', 'POST', input);
  }
  createSession(instanceID: string, model: string) {
    return this.request<Session>(`instances/${encodeURIComponent(instanceID)}/sessions`, 'POST', {
      model,
    });
  }
  async messages(sessionID: string, signal?: AbortSignal) {
    return (
      (await this.request<Message[] | null>(
        `sessions/${encodeURIComponent(sessionID)}/messages`,
        'GET',
        undefined,
        signal,
      )) ?? []
    );
  }
  status(sessionID: string, signal?: AbortSignal) {
    return this.request<QueueStatus>(
      `sessions/${encodeURIComponent(sessionID)}/status`,
      'GET',
      undefined,
      signal,
    );
  }
  statistics(sessionID: string, signal?: AbortSignal) {
    return this.request<Statistics>(
      `sessions/${encodeURIComponent(sessionID)}/statistics`,
      'GET',
      undefined,
      signal,
    );
  }
  send(sessionID: string, content: string) {
    return this.request<AcceptedMessage>(
      `sessions/${encodeURIComponent(sessionID)}/messages`,
      'POST',
      { content },
    );
  }
  cancel(sessionID: string) {
    return this.request(`sessions/${encodeURIComponent(sessionID)}/cancel`, 'POST');
  }
  cancelQueued(sessionID: string, messageID: string) {
    return this.request(
      `sessions/${encodeURIComponent(sessionID)}/queue/${encodeURIComponent(messageID)}`,
      'DELETE',
    );
  }
  resolvePermission(requestID: string, kind: 'allow' | 'deny') {
    return this.request(`permissions/${encodeURIComponent(requestID)}`, 'POST', {
      kind,
      scope: 'once',
    });
  }
  resume(instanceID: string) {
    return this.request<Instance>(`instances/${encodeURIComponent(instanceID)}/start`, 'POST');
  }

  eventsURL(sessionID: string, since: number): URL {
    const url = this.url(`sessions/${encodeURIComponent(sessionID)}/events`);
    url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
    url.searchParams.set('since', String(since));
    if (this.token) url.searchParams.set('token', this.token);
    return url;
  }
}

export function isHarnessEvent(value: unknown): value is HarnessEvent {
  if (!value || typeof value !== 'object') return false;
  return (
    'Seq' in value &&
    typeof value.Seq === 'number' &&
    'Name' in value &&
    typeof value.Name === 'string' &&
    'SessionID' in value &&
    typeof value.SessionID === 'string'
  );
}
