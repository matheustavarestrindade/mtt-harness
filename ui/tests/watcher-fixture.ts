import type { Page } from '@playwright/test';
import type { WatcherConfiguration, WatcherDefinition, WatcherRun } from '../src/lib/atoms/watcher';
import type { WorkspacePluginState } from '../src/lib/atoms/plugins';
import type { PluginActivityPage } from '../src/lib/atoms/plugin-activity';

export async function mockWatchers(page: Page) {
  const states = new Map<string, WorkspacePluginState<WatcherConfiguration>>();
  const activity = new Map<string, PluginActivityPage>();
  const runs = new Map<string, WatcherRun[]>();
  const updates: { workspace: string; patch: Partial<WatcherConfiguration> }[] = [];
  const actions: { workspace: string; action: string; input: Record<string, unknown> }[] = [];
  const defaults: WatcherDefinition = {
    id: '',
    name: '',
    enabled: false,
    model: '',
    reasoning_effort: '',
    prompt:
      'Review {todo} {doing}.\n<conversation_to_analize>{last_context}</conversation_to_analize>',
    triggers: ['end_turn'],
    interval_tokens: 32768,
    context_tokens: 32768,
    include_reasoning: false,
    permission_tools: [],
    outside_workspace_only: false,
    tools: [],
    timeout_ms: 120000,
    max_rounds: 12,
    max_output_tokens: 2048,
    max_continuations: 3,
  };
  const tools = [
    {
      name: 'file_actions',
      description: 'File operations',
      categories: ['file'],
      operations: ['read', 'list', 'glob', 'write', 'replace', 'append', 'prepend', 'delete'],
    },
    { name: 'search_memory', description: 'Read workspace memories', categories: ['memory'] },
    {
      name: 'list_memory_categories',
      description: 'Read memory categories',
      categories: ['memory'],
    },
    { name: 'bash', description: 'Run commands', categories: ['process'] },
  ];
  await page.route('**/api/**/plugins/watcher/**', async (route) => {
    const url = new URL(route.request().url()),
      parts = url.pathname.replace(/^\/api\//, '').split('/');
    const reply = (body: unknown, status = 200) =>
      route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });
    if (parts[0] === 'sessions')
      return reply(
        activity.get(parts[1]) ?? {
          Name: 'watcher',
          SessionID: parts[1],
          Revision: 0,
          HistoryRevision: 0,
          Entries: [],
        },
      );
    const workspace = parts[1];
    if (!states.has(workspace))
      states.set(workspace, {
        Name: 'watcher',
        Version: '0.1.0',
        WorkspaceID: workspace,
        Available: true,
        Enabled: false,
        RequestedEnabled: false,
        Pending: false,
        Error: '',
        Configuration: { enabled: false, watchers: [] },
        Override: {},
        Schema: { JSON: { 'x-tools': tools, 'x-default-definition': defaults } },
      });
    const state = states.get(workspace)!;
    if (parts[4] === 'statistics')
      return reply({
        Name: 'watcher',
        WorkspaceID: workspace,
        Counters: { completed: runs.get(workspace)?.length ?? 0 },
        Agents: [],
      });
    if (parts[4] === 'settings') {
      if (route.request().method() === 'PATCH') {
        const patch = route.request().postDataJSON() as Partial<WatcherConfiguration>;
        updates.push({ workspace, patch });
        state.Configuration = { ...state.Configuration, ...patch };
        state.Enabled = state.RequestedEnabled = state.Configuration.enabled;
        state.Override = state.Configuration;
      }
      return reply(state);
    }
    if (parts[4] === 'actions') {
      const action = parts[5],
        input = route.request().postDataJSON() as Record<string, unknown>;
      actions.push({ workspace, action, input });
      if (action === 'preview')
        return reply({
          prompt:
            '<todo>Saved TODO</todo>\n<conversation_to_analize>Supplied context</conversation_to_analize>',
          input_tokens: 1500,
          input_limit: 32000,
          estimated: true,
          truncated: false,
        });
      if (action === 'runs') return reply({ runs: runs.get(workspace) ?? [] });
      if (action === 'cancel') {
        for (const run of runs.get(workspace) ?? []) {
          if (run.id === input.run_id) run.status = 'cancelled';
        }
        return reply({ cancelled: true });
      }
    }
    return reply({ error: 'unknown watcher route' }, 404);
  });
  return { states, activity, runs, updates, actions, defaults, tools };
}
