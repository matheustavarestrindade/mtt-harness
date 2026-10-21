import type { Page, WebSocketRoute } from '@playwright/test';
import type { ProcessRecord } from '../src/lib/atoms/processes';
import type { MemoryMetrics, MemoryPluginState } from '../src/lib/atoms/memory';
import type { RepetitionConfiguration, WorkspacePluginState } from '../src/lib/atoms/plugins';
import type { PluginActivityPage } from '../src/lib/atoms/plugin-activity';
import type { HistoryPage, SessionViewState } from '../src/lib/atoms/session-view';
import type {
  HarnessEvent,
  Instance,
  Message,
  Model,
  QueueStatus,
  Session,
  Statistics,
  TaskState,
} from '../src/lib/atoms/types';

// Fixtures use the actual Go response casing, nullable lists and 202 queue
// envelope. They are browser-test data, never a fallback in the shipped UI.
export async function mockHarness(page: Page, options: { replayEvents?: boolean } = {}) {
  const instances: Instance[] = [];
  const unlinkedInstances: string[] = [];
  const sessions: Session[] = [];
  const histories = new Map<string, Message[]>();
  const processes = new Map<string, ProcessRecord>();
  const processSockets = new Map<WebSocketRoute, string>();
  const processConnections: string[] = [];
  const processControls: { id: string; control: Record<string, unknown> }[] = [];
  const processOutput = new Map<
    string,
    { stdout: Buffer; stderr: Buffer; stdoutBytes: number; stderrBytes: number }
  >();
  const queued = new Map<string, Message[]>();
  const running = new Set<string>();
  const taskStates = new Map<string, TaskState>();
  const memoryStates = new Map<string, MemoryPluginState>();
  const memoryMetrics = new Map<string, MemoryMetrics>();
  const memoryUpdates: { workspaceID: string; patch: Record<string, unknown> }[] = [];
  const repetitionStates = new Map<string, WorkspacePluginState<RepetitionConfiguration>>();
  const repetitionMetrics = new Map<string, MemoryMetrics>();
  const repetitionUpdates: { workspaceID: string; patch: Partial<RepetitionConfiguration> }[] = [];
  const sidekickStates = new Map<
    string,
    WorkspacePluginState<import('../src/lib/atoms/plugins').SidekickConfiguration>
  >();
  const sidekickMetrics = new Map<string, MemoryMetrics>();
  const sidekickActivity = new Map<string, PluginActivityPage>();
  const sidekickUpdates: { workspaceID: string; patch: Record<string, unknown> }[] = [];
  const settings = new Map<string, Record<string, string>>([
    ['', { agent_depth_limit: '2', process_limit: '8' }],
  ]);
  const sessionUsage: Statistics = {
    Calls: 1,
    Input: 20,
    CacheRead: 0,
    CacheWrite: 0,
    Output: 12,
    Reasoning: 0,
    Cost: null,
    Costs: null,
    CacheHitRate: 0,
    CacheHitPercentage: 0,
  };
  const workspaceUsage: Statistics = {
    ...sessionUsage,
    Calls: 4,
    Input: 1000,
    CacheRead: 500,
    CacheWrite: 100,
    Output: 200,
    Reasoning: 30,
    Cost: { Currency: 'USD', Value: 0.125 },
    Costs: [{ Currency: 'USD', Value: 0.125 }],
    CacheHitRate: 0.3125,
    CacheHitPercentage: 31.25,
  };
  const harnessUsage: Statistics = {
    ...workspaceUsage,
    Calls: 10,
    Input: 3000,
    Output: 900,
    Costs: [
      { Currency: 'USD', Value: 0.75 },
      { Currency: 'EUR', Value: 0.5 },
    ],
    Cost: null,
  };
  const openSockets = new Set<WebSocketRoute>();
  const eventHistory: HarnessEvent[] = [];
  let identifier = 0;
  let sequence = 0;
  let messageSequence = 0;
  const messageSequences = new Map<string, number>();
  const model: Model = {
    ID: 'test/test-model',
    Level: 0,
    Input: ['text'],
    Output: ['text'],
    Tools: true,
    ContextMax: 128000,
    Prices: null,
  };
  const models: Model[] = [model];
  function historyPage(
    session: Session,
    query: URLSearchParams = new URLSearchParams(),
  ): HistoryPage {
    const all = (histories.get(session.ID) ?? []).map((message) => {
      if (!messageSequences.has(message.ID)) {
        messageSequence = Math.max(messageSequence + 1, message.Seq || 0);
        messageSequences.set(message.ID, messageSequence);
      }
      return { ...message, Seq: messageSequences.get(message.ID)! };
    });
    const modelMap: Record<string, string> = {};
    for (const event of eventHistory) {
      const payload = event.Payload as Record<string, unknown> | null;
      if (
        event.SessionID === session.ID &&
        event.Name === 'model.call' &&
        typeof payload?.message_id === 'string' &&
        typeof payload.model === 'string'
      )
        modelMap[payload.message_id] = payload.model;
    }
    const before = Number(query.get('before') ?? 0),
      after = Number(query.get('after') ?? 0),
      limit = Number(query.get('limit') ?? 50);
    const filtered = all.filter(
      (message) => (!before || message.Seq < before) && (!after || message.Seq > after),
    );
    const messages =
      query.get('metadata') === 'true'
        ? []
        : after
          ? filtered.slice(0, limit)
          : filtered.slice(-limit);
    const context: Message[] = [];
    if (messages[0]?.Role === 'tool') {
      const owner = [...all]
        .reverse()
        .find(
          (message) =>
            message.Seq < messages[0].Seq &&
            message.Role === 'assistant' &&
            message.ToolCalls?.some((call) => call.ID === messages[0].ToolCallID),
        );
      if (owner)
        context.push({
          ...owner,
          Content: [],
          Reasoning: '',
          Usage: null,
          ToolCalls:
            owner.ToolCalls?.filter((call) =>
              messages.some((message) => message.ToolCallID === call.ID),
            ) ?? [],
        });
    }
    const latest = [...all].reverse().find((message) => message.Role === 'assistant');
    const ids = new Set([...messages, ...context].map((message) => message.ID));
    return {
      SessionID: session.ID,
      Revision: session.HistoryRevision ?? 0,
      Messages: messages,
      Context: context,
      Models: modelMap,
      Sizes: Object.fromEntries(
        messages.map((message) => [message.ID, JSON.stringify(message).length]),
      ),
      FirstSeq: messages[0]?.Seq ?? 0,
      LastSeq: messages.at(-1)?.Seq ?? 0,
      HeadSeq: all.at(-1)?.Seq ?? 0,
      HasOlder: !!messages.length && all[0].Seq < messages[0].Seq,
      HasNewer: !!messages.length && all.at(-1)!.Seq > messages.at(-1)!.Seq,
      LatestResponse: latest
        ? {
            MessageID: latest.ID,
            Seq: latest.Seq,
            Model: modelMap[latest.ID] ?? '',
            Usage: latest.Usage,
            CreatedAt: latest.CreatedAt,
          }
        : null,
      Processes: [...processes.values()].filter(
        (process) => process.SessionID === session.ID && ids.has(process.Spec.ToolMessageID ?? ''),
      ),
      Task: session.Parent
        ? all
            .find((message) => message.Role === 'user')
            ?.Content?.filter((content) => content.Type === 'text')
            .map((content) => content.Text)
            .join('\n')
        : '',
    };
  }
  function viewState(session: Session, details = true): SessionViewState {
    const history = historyPage(session, new URLSearchParams('metadata=true'));
    const events = eventHistory.filter((event) => event.SessionID === session.ID);
    const permissions = new Map<string, import('../src/lib/atoms/types').PermissionRequest>();
    let live: SessionViewState['Live'] = null;
    let outcome = '';
    for (const event of events) {
      const payload = event.Payload as Record<string, unknown> | null;
      if (event.Name === 'permission.request' && typeof payload?.ID === 'string')
        permissions.set(
          payload.ID,
          payload as unknown as import('../src/lib/atoms/types').PermissionRequest,
        );
      if (event.Name === 'permission.decision' && typeof payload?.RequestID === 'string')
        permissions.delete(payload.RequestID);
      if (event.Name === 'model.call' && typeof payload?.message_id === 'string')
        live = {
          Message: {
            ID: payload.message_id,
            SessionID: session.ID,
            Seq: 0,
            Role: 'assistant',
            Content: [],
            ToolCalls: [],
            ToolCallID: '',
            Usage: null,
            CreatedAt: event.Time,
          },
          Model: String(payload.model ?? ''),
          Revision: 0,
          Truncated: false,
        };
      if (event.Name === 'model.chunk' && live && payload?.message_id === live.Message.ID) {
        if (typeof payload.text === 'string') {
          const text = (live.Message.Content?.[0]?.Text ?? '') + payload.text;
          live.Message.Content = [
            { Type: 'text', Text: text, Data: null, MIME: '', URL: '', Filename: '', AudioID: '' },
          ];
        }
        if (typeof payload.reasoning === 'string')
          live.Message.Reasoning = (live.Message.Reasoning ?? '') + payload.reasoning;
        live.Revision = Number(payload.chunk_index ?? live.Revision + 1);
      }
      if (event.Name === 'action.received' && live) {
        const call = (payload?.Call ??
          payload) as unknown as import('../src/lib/atoms/types').ToolCall;
        if (call?.ID) live.Message.ToolCalls = [...(live.Message.ToolCalls ?? []), call];
      }
      if (['turn.end', 'run.error', 'run.cancelled'].includes(event.Name)) {
        live = null;
        permissions.clear();
        outcome =
          typeof payload?.status === 'string'
            ? payload.status
            : event.Name === 'run.error'
              ? 'error'
              : event.Name === 'run.cancelled'
                ? 'cancelled'
                : 'completed';
      }
    }
    if (live && histories.get(session.ID)?.some((message) => message.ID === live?.Message.ID))
      live = null;
    const instance = instances.find((instance) => instance.ID === session.InstanceID);
    return {
      Session: session,
      Outcome: outcome,
      Status: {
        running: running.has(session.ID),
        queued: queued.get(session.ID)?.length ?? 0,
        messages: queued.get(session.ID)?.map((message) => message.ID) ?? [],
        error: '',
      },
      QueuedMessages: details
        ? (queued.get(session.ID) ?? []).map((message) => {
            let budget = 4096,
              truncated = false;
            const take = (value: string) => {
              let result = '';
              for (const character of value) {
                const bytes = Buffer.byteLength(character);
                if (bytes > budget) {
                  truncated = true;
                  break;
                }
                budget -= bytes;
                result += character;
              }
              return result;
            };
            const content: Message['Content'] = [];
            for (const item of message.Content ?? []) {
              if (content.length >= 32 || budget < 128) {
                truncated = true;
                break;
              }
              budget -= 128;
              const MIME = take(item.MIME),
                Filename = take(item.Filename),
                Text = item.Type === 'text' ? take(item.Text) : '';
              if (item.Type !== 'text' || item.Data || item.URL || item.AudioID) truncated = true;
              content.push({
                Type: item.Type,
                Text,
                Filename,
                MIME,
                Data: null,
                URL: '',
                AudioID: '',
              });
            }
            return {
              ID: message.ID,
              SessionID: message.SessionID,
              Role: message.Role,
              CreatedAt: message.CreatedAt,
              Content: content,
              Truncated: truncated,
            };
          })
        : undefined,
      Statistics: sessionUsage,
      TaskState: taskStates.get(session.ID) ?? {
        SessionID: session.ID,
        Todo: [],
        Doing: null,
        Revision: 0,
        UpdatedAt: session.CreatedAt,
      },
      Permissions: [...permissions.values()],
      History: history,
      Live: details ? live : null,
      Processes: details
        ? [...processes.values()].filter((process) => process.SessionID === session.ID).slice(-128)
        : [],
      WorkspaceLinked: !!instance,
      WorkspaceStopped: instance?.Stopped ?? true,
      ActiveSessions: [...running].filter((identifier) =>
        sessions.some(
          (entry) => entry.ID === identifier && entry.InstanceID === session.InstanceID,
        ),
      ),
    };
  }
  function sendState(
    socket: WebSocketRoute,
    initial = false,
    patch: Partial<SessionViewState> = {},
  ) {
    const url = new URL(socket.url());
    const session = sessions.find((session) => url.pathname.includes(`/sessions/${session.ID}/`));
    if (!session) return;
    socket.send(
      JSON.stringify({
        type: 'state',
        session_id: session.ID,
        cursor: sequence,
        covered_cursor: sequence,
        state: { ...viewState(session, url.searchParams.get('details') !== 'false'), ...patch },
        initial,
        recent: initial
          ? eventHistory
              .filter(
                (event) =>
                  event.SessionID === session.ID &&
                  event.Name !== 'model.chunk' &&
                  event.Name !== 'process.output',
              )
              .slice(-80)
          : undefined,
      }),
    );
  }
  function state(session: Session, patch: Partial<SessionViewState> = {}) {
    for (const socket of openSockets) {
      const url = new URL(socket.url());
      if (
        url.searchParams.get('view') === 'true' &&
        url.pathname.includes(`/sessions/${session.ID}/`)
      )
        sendState(socket, false, patch);
    }
  }
  function outputFrame(
    identifier: string,
    stream: string,
    data: Buffer = Buffer.alloc(0),
    offset = 0,
    snapshot = false,
  ) {
    return JSON.stringify({
      process_id: identifier,
      stream,
      data: data.toString('utf8'),
      data_base64: data.toString('base64'),
      offset,
      snapshot,
      at: new Date().toISOString(),
      error: '',
      exit: stream === 'exit' ? processes.get(identifier)?.Exit : null,
      terminal: stream === 'start' ? processes.get(identifier)?.Spec.Terminal : undefined,
      input:
        stream === 'start'
          ? processes.get(identifier)?.Spec.Terminal === true &&
            processes.get(identifier)?.Status === 'running'
          : undefined,
    });
  }
  await page.routeWebSocket(/\/api\/processes\/.*\/output/, (socket) => {
    const identifier = new URL(socket.url()).pathname.split('/')[3];
    processConnections.push(identifier);
    processSockets.set(socket, identifier);
    socket.onClose((code, reason) => {
      processSockets.delete(socket);
      void socket.close({ code, reason });
    });
    socket.onMessage((message) => {
      const control = JSON.parse(String(message));
      processControls.push({ id: identifier, control });
      if (control.type === 'input') {
        const bytes = control.data_base64
          ? Buffer.from(control.data_base64, 'base64')
          : Buffer.from(control.data ?? '');
        if (bytes.equals(Buffer.from([3]))) processExit(identifier, 130);
        else processChunk(identifier, 'stdout', bytes);
      }
      if (control.type === 'signal' && control.signal === 'interrupt') processExit(identifier, 130);
    });
    socket.send(outputFrame(identifier, 'start'));
    const output = processOutput.get(identifier);
    if (output)
      for (const stream of ['stdout', 'stderr'] as const)
        if (output[stream].length)
          socket.send(
            outputFrame(
              identifier,
              stream,
              output[stream],
              output[`${stream}Bytes`] - output[stream].length,
              true,
            ),
          );
    if (processes.get(identifier)?.Status === 'stopped') {
      socket.send(outputFrame(identifier, 'exit'));
      socket.close();
    }
  });
  function processChunk(
    identifier: string,
    stream: 'stdout' | 'stderr',
    data: string | Uint8Array,
  ) {
    const bytes = Buffer.from(data);
    const output = processOutput.get(identifier) ?? {
      stdout: Buffer.alloc(0),
      stderr: Buffer.alloc(0),
      stdoutBytes: 0,
      stderrBytes: 0,
    };
    const offset = output[`${stream}Bytes`];
    output[`${stream}Bytes`] += bytes.length;
    output[stream] = Buffer.concat([output[stream], bytes]).subarray(-262144);
    processOutput.set(identifier, output);
    for (const [socket, process] of processSockets)
      if (process === identifier) socket.send(outputFrame(identifier, stream, bytes, offset));
  }
  function processExit(identifier: string, code = 0) {
    const record = processes.get(identifier)!;
    record.Status = 'stopped';
    record.Exit = { Code: code, Error: code ? `exit status ${code}` : '', Signal: '' };
    record.EndedAt = new Date().toISOString();
    const session = sessions.find((session) => session.ID === record.SessionID)!;
    event(session, 'process.exit', record);
    for (const [socket, process] of processSockets)
      if (process === identifier) {
        socket.send(outputFrame(identifier, 'exit'));
        processSockets.delete(socket);
        socket.close();
      }
  }
  function disconnectProcess(identifier: string) {
    for (const [socket, process] of processSockets)
      if (process === identifier) {
        processSockets.delete(socket);
        socket.close();
      }
  }
  await page.routeWebSocket(/\/api\/sessions\/.*\/events/, (socket) => {
    openSockets.add(socket);
    socket.onClose(() => openSockets.delete(socket));
    if (new URL(socket.url()).searchParams.get('view') === 'true') {
      sendState(socket, true);
      return;
    }
    if (options.replayEvents) {
      const url = new URL(socket.url());
      const since = Number(url.searchParams.get('since') ?? '0');
      for (const event of eventHistory) {
        if (url.pathname.includes(`/sessions/${event.SessionID}/`) && event.Seq > since)
          socket.send(JSON.stringify(event));
      }
    }
  });
  function event(session: Session, name: string, payload: unknown) {
    const data = payload as Record<string, unknown> | null;
    if (name === 'agent.start' && typeof data?.session === 'string') {
      const child = sessions.find((entry) => entry.ID === data.session);
      if (child && !child.Completed) {
        running.add(child.ID);
        const call = [...eventHistory]
          .reverse()
          .find((event) => event.SessionID === session.ID && event.Name === 'model.call');
        child.ParentMessageID = String(
          data.message_id ?? (call?.Payload as Record<string, unknown>)?.message_id ?? '',
        );
      }
    }
    if (name === 'agent.end' && typeof data?.session === 'string') running.delete(data.session);
    if (
      name === 'turn.start' ||
      (name === 'model.call' &&
        typeof data?.message_id === 'string' &&
        !histories.get(session.ID)?.some((message) => message.ID === data.message_id))
    )
      running.add(session.ID);
    if (['turn.end', 'run.cancelled', 'run.error', 'run.interrupted'].includes(name))
      running.delete(session.ID);
    if (name === 'model.chunk' && data && typeof data.chunk_index !== 'number')
      payload = {
        ...data,
        chunk_index:
          eventHistory.filter(
            (event) =>
              event.SessionID === session.ID &&
              event.Name === 'model.chunk' &&
              (event.Payload as Record<string, unknown>)?.message_id === data.message_id,
          ).length + 1,
      };
    const value: HarnessEvent = {
      Seq: ++sequence,
      Name: name,
      InstanceID: session.InstanceID,
      SessionID: session.ID,
      Payload: payload,
      Time: new Date().toISOString(),
    };
    eventHistory.push(value);
    for (const socket of openSockets) {
      if (socket.url().includes(`/sessions/${session.ID}/`)) {
        const modern = new URL(socket.url()).searchParams.get('view') === 'true';
        socket.send(JSON.stringify(modern ? { type: 'event', event: value } : value));
      }
    }
    if (name !== 'model.chunk' && name !== 'process.output')
      setTimeout(() => {
        for (const entry of sessions.filter((entry) => entry.InstanceID === session.InstanceID))
          state(entry);
      }, 75);
  }
  await page.route(
    (url) => url.pathname.startsWith('/api/'),
    async (route) => {
      const request = route.request();
      const path = new URL(request.url()).pathname.replace(/^\/api/, '');
      const method = request.method();
      const reply = (body: unknown, status = 200) =>
        route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });
      if (path === '/health') return reply({ status: 'ok' });
      if (request.headers().authorization !== 'Bearer test-token')
        return reply({ error: 'the token is not correct' }, 401);
      if (path === '/providers')
        return reply([
          { Name: 'test', APIURL: '', ModelListURL: '', PriceTableURL: '', Interval: 0 },
        ]);
      if (path === '/providers/test/models') return reply([{ ...model, ID: 'test-model' }]);
      if (path === '/instances' && method === 'GET')
        return reply(instances.length ? instances : null);
      if (path === '/instances' && method === 'POST') {
        const input = request.postDataJSON();
        const instance: Instance = {
          ID: `instance-${++identifier}`,
          Workspace: input.workspace,
          Models: null,
          DefaultModel: input.default_model,
          ProcessLimit: 0,
          AgentDepthLimit: 0,
          Stopped: false,
          CreatedAt: new Date().toISOString(),
        };
        instances.push(instance);
        return reply(instance, 201);
      }
      const parts = path.split('/').filter(Boolean);
      if (parts[0] === 'instances' && parts[2] === 'unlink' && method === 'POST') {
        const index = instances.findIndex((instance) => instance.ID === parts[1]);
        if (index < 0) return reply({ error: 'workspace is not linked' }, 404);
        const busy =
          sessions.some(
            (session) =>
              session.InstanceID === parts[1] &&
              (running.has(session.ID) || (queued.get(session.ID)?.length ?? 0) > 0),
          ) ||
          [...processes.values()].some(
            (process) => process.InstanceID === parts[1] && process.Status === 'running',
          );
        if (busy) return reply({ error: 'workspace has active or queued work' }, 409);
        unlinkedInstances.push(parts[1]);
        instances.splice(index, 1);
        return reply({ id: parts[1], status: 'unlinked' });
      }
      if (parts[0] === 'instances' && parts[2] === 'plugins' && parts[3] === 'sidekick') {
        const workspaceID = parts[1];
        if (!sidekickStates.has(workspaceID))
          sidekickStates.set(workspaceID, {
            Name: 'sidekick',
            Version: '0.1.0',
            WorkspaceID: workspaceID,
            Available: true,
            Enabled: false,
            RequestedEnabled: false,
            Pending: false,
            Override: {},
            Configuration: {
              enabled: false,
              worker_model: '',
              worker_effort: '',
              memory_enabled: true,
              files_enabled: true,
              debounce_ms: 750,
              cooldown_ms: 15000,
              job_timeout_ms: 45000,
              worker_count: 1,
              worker_output_tokens: 768,
              memory_limit: 5,
              memory_bytes: 6000,
              file_limit: 5,
              file_bytes: 8000,
              source_bytes: 4000,
              hint_bytes: 1600,
            },
          });
        if (!sidekickMetrics.has(workspaceID))
          sidekickMetrics.set(workspaceID, {
            Name: 'sidekick',
            WorkspaceID: workspaceID,
            Counters: {},
            Agents: [],
          });
        if (parts[4] === 'statistics') return reply(sidekickMetrics.get(workspaceID));
        const state = sidekickStates.get(workspaceID)!;
        if (method === 'PATCH') {
          const patch = request.postDataJSON();
          sidekickUpdates.push({ workspaceID, patch });
          state.Configuration = { ...state.Configuration, ...patch };
          state.Override = { ...state.Override, ...patch };
          state.RequestedEnabled = state.Configuration.enabled;
          if (!state.Pending) state.Enabled = state.RequestedEnabled;
        }
        return reply(state);
      }
      if (parts[0] === 'instances' && parts[2] === 'plugins' && parts[3] === 'spaced_repetition') {
        const workspaceID = parts[1];
        if (!repetitionStates.has(workspaceID))
          repetitionStates.set(workspaceID, {
            Name: 'spaced_repetition',
            Version: '0.1.0',
            WorkspaceID: workspaceID,
            Available: true,
            Enabled: false,
            RequestedEnabled: false,
            Pending: false,
            Override: {},
            Configuration: {
              enabled: false,
              interval: {
                mode: 'model_fraction',
                tokens: 32768,
                fraction: 0.125,
                max_tokens: 32768,
              },
              pattern: ['low', 'low', 'low', 'medium'],
              worker_model: '',
              worker_effort: '',
              worker_output_tokens: 1024,
              job_timeout_ms: 120000,
              max_queries: 4,
              memory_result_limit: 5,
              memory_bytes: 12000,
              source_bytes: 8000,
              worker_count: 2,
            },
          });
        if (!repetitionMetrics.has(workspaceID))
          repetitionMetrics.set(workspaceID, {
            Name: 'spaced_repetition',
            WorkspaceID: workspaceID,
            Counters: {},
            Agents: [],
          });
        if (parts[4] === 'statistics') return reply(repetitionMetrics.get(workspaceID));
        const state = repetitionStates.get(workspaceID)!;
        if (method === 'PATCH') {
          const patch = request.postDataJSON();
          repetitionUpdates.push({ workspaceID, patch });
          state.Configuration = { ...state.Configuration, ...patch };
          state.Override = { ...state.Override, ...patch };
          state.RequestedEnabled = state.Configuration.enabled;
          if (!state.Pending) state.Enabled = state.RequestedEnabled;
        }
        return reply(state);
      }
      if (parts[0] === 'instances' && parts[2] === 'plugins' && parts[3] === 'context') {
        const workspaceID = parts[1];
        if (!memoryStates.has(workspaceID))
          memoryStates.set(workspaceID, {
            Name: 'context',
            Version: '0.1.0',
            WorkspaceID: workspaceID,
            Available: true,
            Enabled: false,
            RequestedEnabled: false,
            Pending: false,
            Configuration: {
              enabled: false,
              worker_model: '',
              worker_effort: '',
              historian_model: '',
            },
            Override: {},
          });
        if (!memoryMetrics.has(workspaceID))
          memoryMetrics.set(workspaceID, {
            Name: 'context',
            WorkspaceID: workspaceID,
            Counters: {},
            Agents: [],
          });
        if (parts[4] === 'statistics') return reply(memoryMetrics.get(workspaceID));
        const state = memoryStates.get(workspaceID)!;
        if (method === 'PATCH') {
          const patch = request.postDataJSON();
          memoryUpdates.push({ workspaceID, patch });
          state.Configuration = { ...state.Configuration, ...patch };
          state.Override = { ...state.Override, ...patch };
          state.RequestedEnabled = state.Configuration.enabled;
          if (!state.Pending) state.Enabled = state.RequestedEnabled;
        }
        return reply(state);
      }
      if (path === '/statistics') return reply(harnessUsage);
      if (path === '/usage') {
        const query = new URL(request.url()).searchParams;
        const statistics = query.has('session_id')
          ? sessionUsage
          : query.has('instance_id')
            ? workspaceUsage
            : harnessUsage;
        const to = query.get('to') ?? new Date().toISOString().slice(0, 10),
          from = query.get('from') ?? to;
        const priced = statistics.Cost || statistics.Costs?.length ? statistics.Calls : 0;
        return reply({
          From: from,
          To: to,
          Timezone: 'UTC',
          Statistics: statistics,
          PricedCalls: priced,
          FailedCalls: 0,
          Buckets: statistics.Calls
            ? [
                {
                  Date: to,
                  ModelID: 'test/test-model',
                  Statistics: statistics,
                  PricedCalls: priced,
                  FailedCalls: 0,
                },
              ]
            : [],
        });
      }
      if (parts[0] === 'settings' || (parts[0] === 'instances' && parts[2] === 'settings')) {
        const scope = parts[0] === 'settings' ? '' : parts[1];
        const key = parts[0] === 'settings' ? parts[1] : parts[3];
        if (method === 'GET') return reply(settings.get(scope) ?? null);
        const values = { ...settings.get(scope) };
        if (method === 'PUT') values[key] = request.postDataJSON().value;
        if (method === 'DELETE') delete values[key];
        settings.set(scope, values);
        return reply({ status: method === 'DELETE' ? 'deleted' : 'saved' });
      }
      if (parts[0] === 'instances' && parts[2] === 'statistics') return reply(workspaceUsage);
      if (parts[0] === 'instances' && parts[2] === 'models') return reply(models);
      if (parts[0] === 'instances' && parts[2] === 'sessions') {
        if (method === 'GET')
          return reply(sessions.filter((session) => session.InstanceID === parts[1]));
        const session: Session = {
          ID: `session-${++identifier}`,
          InstanceID: parts[1],
          Model: request.postDataJSON().model,
          ReasoningEffort: request.postDataJSON().reasoning_effort ?? '',
          Parent: '',
          Depth: 0,
          Completed: false,
          CreatedAt: new Date().toISOString(),
        };
        sessions.push(session);
        histories.set(session.ID, []);
        queued.set(session.ID, []);
        return reply(session, 201);
      }
      const session = sessions.find((entry) => entry.ID === parts[1]);
      if (parts[0] === 'sessions' && session) {
        if (parts.length === 2 && method === 'DELETE') {
          const removed = new Set([session.ID]);
          for (let changed = true; changed;) {
            changed = false;
            for (const entry of sessions) {
              if (
                entry.InstanceID === session.InstanceID &&
                entry.Parent &&
                removed.has(entry.Parent) &&
                !removed.has(entry.ID)
              ) {
                removed.add(entry.ID);
                changed = true;
              }
            }
          }
          const protectedIDs = new Set(removed);
          for (
            let parent = session.Parent;
            parent && !protectedIDs.has(parent);
            parent = sessions.find((entry) => entry.ID === parent)?.Parent ?? ''
          )
            protectedIDs.add(parent);
          if (
            [...protectedIDs].some(
              (identifier) => running.has(identifier) || queued.get(identifier)?.length,
            )
          ) {
            return reply(
              {
                error:
                  'session or child session has active or queued work; stop it before deleting',
              },
              409,
            );
          }
          for (let index = sessions.length - 1; index >= 0; index--) {
            if (removed.has(sessions[index].ID)) sessions.splice(index, 1);
          }
          for (const identifier of removed) {
            histories.delete(identifier);
            queued.delete(identifier);
            running.delete(identifier);
          }
          return reply({ status: 'deleted', session_ids: [...removed] });
        }
        if (parts.length === 2 && method === 'GET') return reply(session);
        if (parts[2] === 'state')
          return reply(
            viewState(session, new URL(request.url()).searchParams.get('details') !== 'false'),
          );
        if (parts[2] === 'history') {
          const query = new URL(request.url()).searchParams;
          if (
            query.has('revision') &&
            Number(query.get('revision')) !== (session.HistoryRevision ?? 0)
          )
            return reply({ error: 'session history changed' }, 409);
          return reply(historyPage(session, query));
        }
        if (parts[2] === 'model' && method === 'PUT') {
          const input = request.postDataJSON();
          const target = models.find((entry) => entry.ID === input.model);
          const current = models.find((entry) => entry.ID === session.Model);
          if (!target || target.ContextMax <= 0)
            return reply({ error: 'Model context size is unavailable' }, 400);
          if ((!current || target.ContextMax < current.ContextMax) && !input.allow_compaction)
            return reply(
              {
                code: 'context_compaction_required',
                error: 'The conversation context will be compacted to fit.',
                model: target.ID,
                current_context_max: current?.ContextMax ?? 0,
                target_context_max: target.ContextMax,
              },
              409,
            );
          session.Model = target.ID;
          session.SelectionRevision = (session.SelectionRevision ?? 0) + 1;
          if (
            !target.Reasoning ||
            !target.ReasoningEfforts?.includes(session.ReasoningEffort ?? '')
          )
            session.ReasoningEffort = '';
          setTimeout(() => state(session), 75);
          return reply(session);
        }
        if (parts[2] === 'reasoning' && method === 'PUT') {
          const effort = request.postDataJSON().effort;
          const current = models.find((entry) => entry.ID === session.Model);
          if (
            typeof effort !== 'string' ||
            (effort && !current?.ReasoningEfforts?.includes(effort))
          )
            return reply({ error: 'Unsupported reasoning effort' }, 400);
          session.ReasoningEffort = effort;
          session.SelectionRevision = (session.SelectionRevision ?? 0) + 1;
          setTimeout(() => state(session), 75);
          return reply(session);
        }
        if (parts[2] === 'messages' && method === 'GET') return reply(histories.get(session.ID));
        if (parts[2] === 'plugins' && parts[3] === 'sidekick' && parts[4] === 'activity')
          return reply(
            sidekickActivity.get(session.ID) ?? {
              Name: 'sidekick',
              SessionID: session.ID,
              Revision: 0,
              HistoryRevision: 0,
              Entries: [],
            },
          );
        if (parts[2] === 'processes' && method === 'GET')
          return reply(
            [...processes.values()].filter((process) => process.SessionID === session.ID),
          );
        if (parts[2] === 'status')
          return reply({
            running: running.has(session.ID),
            queued: queued.get(session.ID)?.length ?? 0,
            messages: queued.get(session.ID)?.map((message) => message.ID) ?? null,
            error: '',
          } satisfies QueueStatus);
        if (parts[2] === 'statistics') return reply(sessionUsage);
        if (parts[2] === 'task-state')
          return reply(
            taskStates.get(session.ID) ??
              ({
                SessionID: session.ID,
                Todo: [],
                Doing: null,
                Revision: 0,
                UpdatedAt: session.CreatedAt,
              } satisfies TaskState),
          );
        if (parts[2] === 'messages' && method === 'POST') {
          const content = request.postDataJSON().content;
          const message: Message = {
            ID: `message-${++identifier}`,
            SessionID: session.ID,
            Seq: 0,
            Role: 'user',
            Content: Array.isArray(content)
              ? content
              : [
                  {
                    Type: 'text',
                    Text: content,
                    Data: null,
                    MIME: '',
                    URL: '',
                    Filename: '',
                    AudioID: '',
                  },
                ],
            ToolCalls: null,
            ToolCallID: '',
            Usage: null,
            CreatedAt: new Date().toISOString(),
          };
          if (running.has(session.ID)) {
            queued.get(session.ID)?.push(message);
            setTimeout(() => state(session), 75);
            return reply(
              { status: 'queued', position: queued.get(session.ID)?.length, message },
              202,
            );
          }
          histories.get(session.ID)?.push(message);
          const messageText =
            typeof content === 'string'
              ? content
              : content
                  .filter((part: { Type: string; Text: string }) => part.Type === 'text')
                  .map((part: { Text: string }) => part.Text)
                  .join('\n');
          if (messageText.includes('wait')) {
            running.add(session.ID);
            event(session, 'turn.start', null);
          } else {
            histories.get(session.ID)?.push({
              ...message,
              ID: `assistant-${identifier}`,
              Role: 'assistant',
              Content: [
                {
                  Type: 'text',
                  Text: `Done. ${messageText}`,
                  Data: null,
                  MIME: '',
                  URL: '',
                  Filename: '',
                  AudioID: '',
                },
              ],
            });
            event(session, 'turn.end', { status: 'completed' });
          }
          return reply({ status: 'queued', position: 1, message }, 202);
        }
        if (parts[2] === 'queue' && method === 'GET') {
          const message = queued.get(session.ID)?.find((message) => message.ID === parts[3]);
          return message
            ? reply(message)
            : reply({ error: 'message is not pending in the session' }, 404);
        }
        if (parts[2] === 'queue' && method === 'DELETE') {
          queued.set(
            session.ID,
            queued.get(session.ID)?.filter((message) => message.ID !== parts[3]) ?? [],
          );
          setTimeout(() => state(session), 75);
          return reply({ status: 'removed' });
        }
        if (parts[2] === 'cancel') {
          running.delete(session.ID);
          event(session, 'turn.end', { status: 'cancelled' });
          return reply({ status: 'cancelled' });
        }
      }
      if (parts[0] === 'permissions') {
        const owner = sessions.find((candidate) =>
          viewState(candidate).Permissions.some((permission) => permission.ID === parts[1]),
        );
        if (owner) event(owner, 'permission.decision', { RequestID: parts[1] });
        return reply({ status: 'resolved' });
      }
      return reply({ error: `Fixture has no route for ${method} ${path}` }, 404);
    },
  );
  return {
    instances,
    unlinkedInstances,
    sessions,
    histories,
    processes,
    processSockets,
    processConnections,
    processControls,
    processChunk,
    processExit,
    disconnectProcess,
    settings,
    model,
    models,
    sessionUsage,
    queued,
    running,
    event,
    state,
    historyPage,
    viewState,
    taskStates,
    memoryStates,
    memoryMetrics,
    memoryUpdates,
    repetitionStates,
    repetitionMetrics,
    repetitionUpdates,
    sidekickStates,
    sidekickMetrics,
    sidekickActivity,
    sidekickUpdates,
    openSockets,
    eventHistory,
  };
}
