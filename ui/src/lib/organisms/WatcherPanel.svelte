<script lang="ts">
  import { untrack } from 'svelte';
  import { Eye, Plus, Copy, Pencil, Trash2, X } from 'lucide-svelte';
  import { Button } from '../atoms/ui/button';
  import { Input } from '../atoms/ui/input';
  import { Switch } from '../atoms/ui/switch';
  import * as Tabs from '../atoms/ui/tabs';
  import { workspaceName } from '../atoms/format';
  import { createClientID } from '../atoms/identifiers';
  import type { Instance, Session, Model } from '../atoms/types';
  import type {
    WatcherConfiguration,
    WatcherDefinition,
    WatcherTool,
    WatcherRun,
    WatcherRunPage,
    WatcherPreview,
  } from '../atoms/watcher';
  import { watcherTriggers } from '../atoms/watcher';
  import type { HarnessApi } from '../molecules/api/client';
  import WorkerUsageBreakdown from '../molecules/WorkerUsageBreakdown.svelte';
  import PluginPanelFooter from '../molecules/PluginPanelFooter.svelte';
  import SettingsPageTitle from '../molecules/SettingsPageTitle.svelte';
  import Disclosure from '../molecules/Disclosure.svelte';
  import WatcherEditor from './WatcherEditor.svelte';
  import { WorkspacePluginPanelState } from './plugin-panel.svelte';
  let {
    api,
    instance,
    session,
  }: { api: HarnessApi; instance: Instance | null; session: Session | null } = $props();
  const panel = new WorkspacePluginPanelState<WatcherConfiguration>('watcher', 'Watcher');
  const workspaceID = $derived(instance?.ID ?? '');
  let models = $state<Model[]>([]),
    editing = $state<WatcherDefinition | null>(null),
    query = $state(''),
    page = $state(0),
    tab = $state<'definitions' | 'activity' | 'usage'>('definitions');
  let runs = $state<WatcherRun[]>([]),
    cursor = $state(''),
    reading = $state(false),
    activityError = $state(''),
    removeID = $state('');
  let activityUpdatedAt = $state<Date | null>(null);
  let lifetime = new AbortController();
  let scope = 0;
  const tools = $derived((panel.state?.Schema?.JSON?.['x-tools'] ?? []) as WatcherTool[]);
  const definitions = $derived(
    (panel.state?.Configuration.watchers ?? []).filter((value) =>
      value.name.toLowerCase().includes(query.trim().toLowerCase()),
    ),
  );
  const visible = $derived(definitions.slice(page * 20, (page + 1) * 20));
  const disabled = $derived(panel.saving || !panel.state?.Available || !!instance?.Stopped);
  $effect(() => {
    const client = api,
      identifier = workspaceID;
    scope++;
    const version = scope;
    lifetime.abort();
    lifetime = new AbortController();
    const signal = lifetime.signal;
    models = [];
    editing = null;
    runs = [];
    cursor = '';
    reading = false;
    activityError = '';
    activityUpdatedAt = null;
    removeID = '';
    page = 0;
    untrack(() => panel.connect(client, identifier));
    if (identifier)
      void client
        .models(identifier, signal)
        .then((value) => {
          if (!signal.aborted && version === scope) models = value;
        })
        .catch((failure) => {
          if (!signal.aborted)
            activityError = failure instanceof Error ? failure.message : 'Cannot load models.';
        });
    return () => {
      lifetime.abort();
      panel.disconnect();
    };
  });
  $effect(() => {
    if (page * 20 >= definitions.length) page = 0;
  });
  $effect(() => {
    const metrics = panel.metrics;
    const current = tab;
    if (current === 'activity' && metrics) untrack(() => void loadRuns());
  });
  async function configure(watchers: WatcherDefinition[]) {
    return panel.configure({ watchers });
  }
  function createWatcher(copy?: WatcherDefinition) {
    const defaults = panel.state?.Schema?.JSON?.['x-default-definition'] as
      WatcherDefinition | undefined;
    if (!defaults) return;
    const value = structuredClone($state.snapshot(copy ?? defaults));
    value.id = createClientID();
    value.name = copy ? `${copy.name} copy` : '';
    value.enabled = true;
    value.permission_tools ??= [];
    value.tools ??= [];
    value.reasoning_effort ??= '';
    value.model ??= '';
    if (!copy)
      value.tools = tools
        .filter((tool) =>
          ['file_actions', 'search_memory', 'list_memory_categories'].includes(tool.name),
        )
        .map((tool) => ({
          name: tool.name,
          ...(tool.operations?.length
            ? {
                operations: tool.operations.filter((operation) =>
                  ['read', 'list', 'glob'].includes(operation),
                ),
              }
            : {}),
        }));
    editing = value;
    tab = 'definitions';
    removeID = '';
  }
  async function saveWatcher(value: WatcherDefinition) {
    const watchers = panel.state?.Configuration.watchers ?? [];
    const next = watchers.some((item) => item.id === value.id)
      ? watchers.map((item) => (item.id === value.id ? value : item))
      : [...watchers, value];
    const saved = await configure(next);
    if (saved) editing = null;
    return saved;
  }
  async function loadRuns(more = false) {
    if (!workspaceID || reading) return;
    const identifier = workspaceID,
      version = scope,
      signal = lifetime.signal;
    reading = true;
    try {
      const result = await api.pluginAction<WatcherRunPage>(
        identifier,
        'watcher',
        'runs',
        { limit: 50, cursor: more ? cursor : '' },
        signal,
      );
      if (signal.aborted || version !== scope) return;
      if (result.runs.some((run) => run.workspace_id !== identifier))
        throw new Error('Watcher activity belongs to another workspace.');
      runs = (more ? [...runs, ...result.runs] : result.runs).slice(0, 500);
      cursor = runs.length < 500 ? (result.next_cursor ?? '') : '';
      activityError = '';
      activityUpdatedAt = new Date();
    } catch (failure) {
      if (!signal.aborted && version === scope)
        activityError =
          failure instanceof Error ? failure.message : 'Cannot load watcher activity.';
    } finally {
      if (version === scope) reading = false;
    }
  }
  async function cancelRun(identifier: string) {
    const version = scope,
      signal = lifetime.signal,
      workspace = workspaceID;
    try {
      await api.pluginAction(workspace, 'watcher', 'cancel', { run_id: identifier }, signal);
      if (version === scope && !signal.aborted) await loadRuns();
    } catch (failure) {
      if (version === scope && !signal.aborted)
        activityError = failure instanceof Error ? failure.message : 'Cannot cancel review.';
    }
  }
  async function preview(value: WatcherDefinition) {
    if (!session || session.InstanceID !== workspaceID) return null;
    return api.pluginAction<WatcherPreview>(
      workspaceID,
      'watcher',
      'preview',
      { session_id: session.ID, watcher: value, trigger: value.triggers[0] },
      lifetime.signal,
    );
  }
  function selectTab(value: string) {
    if (!['definitions', 'usage', 'activity'].includes(value) || value === tab) return;
    tab = value as typeof tab;
    editing = null;
    if (tab === 'activity') void loadRuns();
  }
  async function refreshWatchers() {
    await panel.refresh();
    if (tab === 'activity') await loadRuns();
  }
  const updatedAt = $derived(tab === 'activity' ? activityUpdatedAt : panel.updatedAt);
</script>

<section aria-label="Watcher settings" class="min-w-0 space-y-5">
  <SettingsPageTitle icon={Eye} title="Watchers" description="Automatic workspace reviews.">
    {#snippet actions()}
      {#if panel.state}<div class="flex min-h-11 shrink-0 items-center gap-3">
          <span role="status" class="text-xs text-muted-foreground"
            >{panel.state.Pending ? 'Updating…' : panel.state.Enabled ? 'On' : 'Off'}</span
          >
          <Switch
            aria-label="Watchers enabled"
            {disabled}
            bind:checked={
              () => panel.state?.RequestedEnabled ?? false,
              (checked) => void panel.configure({ enabled: checked })
            }
          />
        </div>{/if}
    {/snippet}
  </SettingsPageTitle>
  {#if !instance}<p class="text-sm text-muted-foreground">
      Select a workspace to configure watchers.
    </p>{:else}
    <p class="break-words text-xs text-muted-foreground" title={instance.Workspace}>
      {workspaceName(instance.Workspace)} · Workspace settings
    </p>
    {#if panel.error}<p role="alert" class="break-words text-sm text-destructive">
        {panel.error}
      </p>{/if}
    {#if panel.state?.Error}<p role="status" class="break-words text-xs text-muted-foreground">
        {panel.state.Error}
      </p>{/if}
    {#if panel.actionError}<p role="alert" class="break-words text-sm text-destructive">
        {panel.actionError}
      </p>{/if}
    <Tabs.Root bind:value={() => tab, selectTab} class="min-w-0 gap-0">
      <Tabs.List
        aria-label="Watcher sections"
        class="grid h-[50px] min-h-[50px] w-full grid-cols-3"
      >
        <Tabs.Trigger value="definitions" class="min-h-11">Watchers</Tabs.Trigger>
        <Tabs.Trigger value="usage" class="min-h-11">Usage</Tabs.Trigger>
        <Tabs.Trigger value="activity" class="min-h-11">Activity</Tabs.Trigger>
      </Tabs.List>
      <div class="min-w-0 pt-4">
        <Tabs.Content value="definitions" class="m-0 min-h-52 min-w-0 space-y-4">
          <div class="flex min-h-11 items-center justify-between gap-3">
            <h3 class="text-sm font-medium">
              {editing ? 'Watcher configuration' : 'Definitions'}
            </h3>
            {#if !editing}<Button
                variant="outline"
                class="h-11"
                {disabled}
                onclick={() => createWatcher()}><Plus class="size-4" />New watcher</Button
              >{/if}
          </div>
          {#if editing}{#key editing.id}<WatcherEditor
                value={editing}
                {models}
                {tools}
                saving={panel.saving}
                canPreview={!!session && session.InstanceID === workspaceID}
                onSave={saveWatcher}
                onCancel={() => (editing = null)}
                onPreview={preview}
              />{/key}
          {:else}
            {#if definitions.length > 10 || query}<Input
                aria-label="Search watchers"
                placeholder="Search watchers…"
                bind:value={query}
              />{/if}
            <div class="space-y-2">
              {#each visible as value (value.id)}<article
                  class="rounded-md border border-border p-3"
                  aria-label={`Watcher ${value.name}`}
                >
                  <div class="flex min-w-0 items-start gap-3">
                    <div class="min-w-0 flex-1">
                      <h3 class="break-words text-sm font-medium">{value.name}</h3>
                      <p class="mt-1 text-xs text-muted-foreground">
                        {value.triggers
                          .map(
                            (trigger) =>
                              watcherTriggers.find((item) => item.value === trigger)?.label ??
                              trigger,
                          )
                          .join(' · ')}
                      </p>
                    </div>
                    <div class="flex min-h-11 shrink-0 items-center">
                      <Switch
                        aria-label={`Enable watcher ${value.name}`}
                        {disabled}
                        bind:checked={
                          () => value.enabled,
                          (checked) =>
                            void configure(
                              panel.state!.Configuration.watchers.map((item) =>
                                item.id === value.id ? { ...item, enabled: checked } : item,
                              ),
                            )
                        }
                      />
                    </div>
                  </div>
                  <div class="mt-2 flex flex-wrap gap-1">
                    <Button
                      variant="ghost"
                      class="h-11"
                      {disabled}
                      onclick={() => (editing = structuredClone($state.snapshot(value)))}
                      ><Pencil class="size-3.5" />Edit</Button
                    ><Button
                      variant="ghost"
                      class="h-11"
                      {disabled}
                      onclick={() => createWatcher(value)}
                      ><Copy class="size-3.5" />Duplicate</Button
                    ><Button
                      variant="ghost"
                      class="h-11"
                      {disabled}
                      onclick={() => (removeID = value.id)}
                      ><Trash2 class="size-3.5" />Remove</Button
                    >
                  </div>
                  {#if removeID === value.id}<div
                      class="mt-2 flex flex-wrap items-center gap-2 border-t border-border pt-3"
                    >
                      <span class="text-xs"
                        >Remove this watcher definition? Run history is kept.</span
                      ><Button
                        variant="destructive"
                        class="h-11"
                        disabled={panel.saving}
                        onclick={async () => {
                          if (
                            await configure(
                              panel.state!.Configuration.watchers.filter(
                                (item) => item.id !== value.id,
                              ),
                            )
                          )
                            removeID = '';
                        }}>Remove watcher</Button
                      ><Button variant="ghost" class="h-11" onclick={() => (removeID = '')}
                        >Keep</Button
                      >
                    </div>{/if}
                </article>{/each}
            </div>
            {#if !definitions.length}<p class="text-sm text-muted-foreground">
                No watchers configured.
              </p>{/if}
            {#if definitions.length > 20}<div class="flex items-center justify-between gap-3">
                <Button variant="outline" class="h-11" disabled={page === 0} onclick={() => page--}
                  >Previous</Button
                ><span class="text-xs">{page + 1} / {Math.ceil(definitions.length / 20)}</span
                ><Button
                  variant="outline"
                  class="h-11"
                  disabled={(page + 1) * 20 >= definitions.length}
                  onclick={() => page++}>Next</Button
                >
              </div>{/if}
          {/if}
        </Tabs.Content>
        <Tabs.Content value="usage" class="m-0 min-h-52 min-w-0 space-y-4">
          <div class="flex min-h-11 items-center justify-between gap-3">
            <h3 class="text-sm font-medium">Worker usage</h3>
          </div>
          {#if panel.metrics?.Agents.length}<WorkerUsageBreakdown
              agents={panel.metrics.Agents}
              {models}
              agentLabel={(name) =>
                panel.state?.Configuration.watchers.find((value) => `watcher.${value.id}` === name)
                  ?.name ?? name}
            />{/if}
          {#if !panel.metrics?.Agents?.length}<p class="text-sm text-muted-foreground">
              No watcher usage recorded.
            </p>{/if}
        </Tabs.Content>
        <Tabs.Content value="activity" class="m-0 min-h-52 min-w-0 space-y-4">
          <div class="flex min-h-11 items-center justify-between gap-3">
            <h3 class="text-sm font-medium">Recent reviews</h3>
          </div>
          {#if activityError}<p role="alert" class="break-words text-sm text-destructive">
              {activityError}
            </p>{/if}
          <div class="space-y-2">
            {#each runs as run (run.id)}<Disclosure
                class="min-w-0 rounded-md border border-border p-3"
                triggerClass="min-h-8 py-1 text-sm pointer-coarse:min-h-11"
              >
                {#snippet heading()}<span class="min-w-0 break-words">{run.name}</span><span
                    class="ml-2 text-xs text-muted-foreground"
                    >{run.status} · {watcherTriggers.find((value) => value.value === run.trigger)
                      ?.label ?? run.trigger}</span
                  >{/snippet}
                <div class="mt-3 min-w-0 space-y-3 text-xs">
                  <p class="text-muted-foreground">
                    {new Date(run.created_at).toLocaleString()} · {run.duration_ms} ms{run.decision
                      ? ` · ${run.decision}`
                      : ''}
                  </p>
                  {#if run.message || run.reason}<p class="whitespace-pre-wrap break-words">
                      {run.message || run.reason}
                    </p>{/if}{#if run.error}<p class="break-words text-destructive">
                      {run.error}
                    </p>{/if}
                  {#if run.status === 'running'}<Button
                      variant="outline"
                      class="h-11"
                      onclick={() => void cancelRun(run.id)}
                      ><X class="size-4" />Cancel review</Button
                    >{/if}
                  <Disclosure
                    title={`Input · ${run.estimated ? '≈ ' : ''}${run.input_tokens.toLocaleString()} tokens${run.input_truncated ? ' · excerpt' : ''}`}
                  >
                    <pre
                      class="max-h-72 overflow-auto whitespace-pre-wrap break-words">{run.input_preview}</pre>
                  </Disclosure>
                </div>
              </Disclosure>{/each}
          </div>
          {#if !runs.length && !reading}<p class="text-sm text-muted-foreground">
              No watcher runs yet.
            </p>{/if}
          {#if cursor}<Button
              variant="outline"
              class="h-11"
              disabled={reading}
              onclick={() => void loadRuns(true)}>Load earlier runs</Button
            >{/if}
        </Tabs.Content>
      </div>
    </Tabs.Root>
    <PluginPanelFooter
      refreshLabel="Refresh Watchers"
      {updatedAt}
      refreshing={panel.loading || reading}
      disabled={panel.saving}
      onRefresh={() => void refreshWatchers()}
    />
  {/if}
</section>
