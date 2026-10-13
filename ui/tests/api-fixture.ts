import type { Page, WebSocketRoute } from '@playwright/test';
import type { MemoryMetrics, MemoryPluginState } from '../src/lib/atoms/memory';
import type { RepetitionConfiguration, WorkspacePluginState } from '../src/lib/atoms/plugins';
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
export async function mockHarness(page: Page) {
  const instances: Instance[] = [];
  const sessions: Session[] = [];
  const histories = new Map<string, Message[]>();
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
  const sockets: WebSocketRoute[] = [];
  let identifier = 0;
  let sequence = 0;
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
  await page.routeWebSocket(/\/api\/sessions\/.*\/events/, (socket) => {
    sockets.push(socket);
  });
  function event(session: Session, name: string, payload: unknown) {
    const value: HarnessEvent = {
      Seq: ++sequence,
      Name: name,
      InstanceID: session.InstanceID,
      SessionID: session.ID,
      Payload: payload,
      Time: new Date().toISOString(),
    };
    for (const socket of sockets) {
      if (socket.url().includes(`/sessions/${session.ID}/`)) socket.send(JSON.stringify(value));
    }
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
          if (
            !target.Reasoning ||
            !target.ReasoningEfforts?.includes(session.ReasoningEffort ?? '')
          )
            session.ReasoningEffort = '';
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
          return reply(session);
        }
        if (parts[2] === 'messages' && method === 'GET') return reply(histories.get(session.ID));
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
        if (parts[2] === 'queue' && method === 'DELETE') {
          queued.set(
            session.ID,
            queued.get(session.ID)?.filter((message) => message.ID !== parts[3]) ?? [],
          );
          return reply({ status: 'removed' });
        }
        if (parts[2] === 'cancel') {
          running.delete(session.ID);
          event(session, 'turn.end', { status: 'cancelled' });
          return reply({ status: 'cancelled' });
        }
      }
      if (parts[0] === 'permissions') {
        if (session) event(session, 'permission.decision', { RequestID: parts[1] });
        return reply({ status: 'resolved' });
      }
      return reply({ error: `Fixture has no route for ${method} ${path}` }, 404);
    },
  );
  return {
    instances,
    sessions,
    histories,
    settings,
    model,
    models,
    sessionUsage,
    running,
    event,
    taskStates,
    memoryStates,
    memoryMetrics,
    memoryUpdates,
    repetitionStates,
    repetitionMetrics,
    repetitionUpdates,
    sidekickStates,
    sidekickMetrics,
    sidekickUpdates,
  };
}
