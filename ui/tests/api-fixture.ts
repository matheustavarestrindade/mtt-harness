import type { Page, WebSocketRoute } from '@playwright/test';
import type {
  HarnessEvent,
  Instance,
  Message,
  Model,
  QueueStatus,
  Session,
} from '../src/lib/atoms/types';

// Fixtures use the actual Go response casing, nullable lists and 202 queue
// envelope. They are browser-test data, never a fallback in the shipped UI.
export async function mockHarness(page: Page) {
  const instances: Instance[] = [];
  const sessions: Session[] = [];
  const histories = new Map<string, Message[]>();
  const queued = new Map<string, Message[]>();
  const running = new Set<string>();
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
      if (parts[0] === 'instances' && parts[2] === 'models') return reply([model]);
      if (parts[0] === 'instances' && parts[2] === 'sessions') {
        if (method === 'GET')
          return reply(sessions.filter((session) => session.InstanceID === parts[1]));
        const session: Session = {
          ID: `session-${++identifier}`,
          InstanceID: parts[1],
          Model: request.postDataJSON().model,
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
        if (parts[2] === 'messages' && method === 'GET') return reply(histories.get(session.ID));
        if (parts[2] === 'status')
          return reply({
            running: running.has(session.ID),
            queued: queued.get(session.ID)?.length ?? 0,
            messages: queued.get(session.ID)?.map((message) => message.ID) ?? null,
            error: '',
          } satisfies QueueStatus);
        if (parts[2] === 'statistics')
          return reply({
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
          });
        if (parts[2] === 'messages' && method === 'POST') {
          const content = request.postDataJSON().content;
          const message: Message = {
            ID: `message-${++identifier}`,
            SessionID: session.ID,
            Seq: 0,
            Role: 'user',
            Content: [
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
          if (content.includes('wait')) {
            running.add(session.ID);
            event(session, 'turn.start', null);
          } else {
            histories
              .get(session.ID)
              ?.push({
                ...message,
                ID: `assistant-${identifier}`,
                Role: 'assistant',
                Content: [{ ...message.Content![0], Text: `Done. ${content}` }],
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
  return { instances, sessions, histories, event };
}
