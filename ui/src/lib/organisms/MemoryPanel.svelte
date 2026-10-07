<script lang="ts">
  import { untrack } from 'svelte';
  import {
    BrainCircuit,
    RefreshCw,
    Settings2,
    X,
    LoaderCircle,
    Power,
    Database,
    Activity,
  } from 'lucide-svelte';
  import { Button } from '../atoms/ui/button';
  import { Label } from '../atoms/ui/label';
  import type { Instance, Model } from '../atoms/types';
  import { costLabel, formatTokenCount, modelLabel, workspaceName } from '../atoms/format';
  import { memoryCounter, totalAgentUsage, agentTokenCount } from '../atoms/memory';
  import type { HarnessApi } from '../molecules/api/client';
  import MemoryAgentUsage from '../molecules/MemoryAgentUsage.svelte';
  import SelectField from '../molecules/SelectField.svelte';
  import { MemoryPanelState } from './memory-panel.svelte';

  let {
    api,
    instance,
    models,
    onClose,
  }: { api: HarnessApi | null; instance: Instance | null; models: Model[]; onClose: () => void } =
    $props();
  const panel = new MemoryPanelState();
  let settingsOpen = $state(false);
  let workerModel = $state('');
  let enableAfterSave = $state(false);
  const workspaceID = $derived(instance?.ID ?? '');
  const count = (key: string) => memoryCounter(panel.metrics, key);
  const usage = $derived(totalAgentUsage(panel.metrics?.Agents ?? []));
  const memories = $derived(count('memory/active') + count('memory/consolidated'));
  const retained = $derived(
    count('memory/deleted') + count('memory/superseded') + count('memory/invalidated'),
  );
  const costEstimated = $derived(usage.statistics.Costs?.some((cost) => cost.Estimated));
  const status = $derived(
    !panel.state
      ? ''
      : !panel.state.Available
        ? 'Unavailable'
        : panel.state.Pending
          ? panel.state.RequestedEnabled
            ? 'Turning on…'
            : 'Turning off…'
          : panel.state.Enabled
            ? 'On'
            : 'Off',
  );
  const configurationDisabled = $derived(
    panel.saving || !!panel.state?.Pending || !panel.state || !!instance?.Stopped,
  );
  const embeddingChunks = $derived(
    Object.entries(panel.metrics?.Counters ?? {}).reduce(
      (total, [name, value]) => total + (name.endsWith('/embedding_chunks') ? value : 0),
      0,
    ),
  );

  $effect(() => {
    const selectedAPI = api;
    const selectedWorkspace = workspaceID;
    untrack(() => panel.connect(selectedAPI, selectedWorkspace));
    settingsOpen = false;
    const updateVisibility = () => (document.hidden ? panel.pause() : void panel.refresh());
    document.addEventListener('visibilitychange', updateVisibility);
    return () => {
      document.removeEventListener('visibilitychange', updateVisibility);
      panel.disconnect();
    };
  });

  function openConfiguration(enable = false) {
    workerModel = panel.state?.Configuration.worker_model ?? '';
    enableAfterSave = enable;
    panel.actionError = '';
    settingsOpen = true;
  }
  async function toggleMemory() {
    if (!panel.state || configurationDisabled) return;
    if (!panel.state.RequestedEnabled && !panel.state.Available) return;
    if (!panel.state.RequestedEnabled && !panel.state.Configuration.worker_model) {
      openConfiguration(true);
      return;
    }
    await panel.configure({ enabled: !panel.state.RequestedEnabled });
  }
  async function saveConfiguration() {
    if (!workerModel || configurationDisabled || (enableAfterSave && !panel.state?.Available))
      return;
    if (
      await panel.configure({
        worker_model: workerModel,
        ...(workerModel !== panel.state?.Configuration.worker_model ? { worker_effort: '' } : {}),
        ...(enableAfterSave ? { enabled: true } : {}),
      })
    )
      settingsOpen = false;
  }
</script>

<section
  id="workspace-memory-panel"
  aria-label="Workspace memory"
  class="flex h-full min-h-0 flex-col bg-background"
>
  <header class="flex h-14 shrink-0 items-center gap-2 border-b border-border px-4">
    <BrainCircuit class="size-4 text-muted-foreground" />
    <h2 class="flex-1 text-sm font-medium">Memory</h2>
    <Button
      variant="ghost"
      size="icon"
      class="icon-button text-muted-foreground"
      aria-label="Memory settings"
      title="Memory settings"
      aria-expanded={settingsOpen}
      disabled={!panel.state}
      onclick={() => (settingsOpen ? (settingsOpen = false) : openConfiguration())}
      ><Settings2 class="size-3.5" /></Button
    >
    <Button
      variant="ghost"
      size="icon"
      class="icon-button text-muted-foreground"
      aria-label="Close memory panel"
      title="Close memory panel"
      onclick={onClose}><X class="size-4" /></Button
    >
  </header>
  <div class="min-h-0 flex-1 overflow-y-auto overscroll-contain px-4">
    {#if !api || !instance}
      <div class="py-8 text-center text-xs leading-6 text-muted-foreground">
        <Database class="mx-auto mb-3 size-6 opacity-60" />{!api
          ? 'Connect to see workspace memory.'
          : 'Choose a workspace to see its memory.'}
      </div>
    {:else}
      <div class="flex min-h-16 items-center gap-2 border-b border-border py-3">
        <div class="min-w-0 flex-1">
          <p class="truncate text-xs font-medium" title={instance.Workspace}>
            {workspaceName(instance.Workspace)}
          </p>
          <p class="mt-1 text-[10px] text-muted-foreground">Shared across workspace sessions</p>
        </div>
        {#if panel.state}<span
            class="shrink-0 rounded-md bg-muted px-2 py-1 text-[10px] font-medium"
            role="status">{status}</span
          >{/if}
      </div>
      {#if panel.error}<p role="alert" class="my-3 break-words text-xs leading-5 text-destructive">
          {panel.error}{panel.metrics ? ' Showing the last update.' : ''}
        </p>{/if}
      {#if panel.state?.Error}<p
          role="alert"
          class="my-3 break-words text-xs leading-5 text-destructive"
        >
          {panel.state.Error}
        </p>{/if}
      {#if panel.actionError}<p
          role="alert"
          class="my-3 break-words text-xs leading-5 text-destructive"
        >
          {panel.actionError}
        </p>{/if}
      {#if settingsOpen && panel.state}
        <form
          class="space-y-3 border-b border-border py-4"
          onsubmit={(event) => {
            event.preventDefault();
            void saveConfiguration();
          }}
        >
          <div class="flex items-center justify-between gap-2">
            <h3 class="text-xs font-medium">Workspace settings</h3>
            <Button
              type="button"
              variant="outline"
              class="h-9 gap-1.5 px-2 text-xs"
              disabled={configurationDisabled ||
                (!panel.state.RequestedEnabled && !panel.state.Available)}
              aria-label={panel.state.RequestedEnabled ? 'Disable memory' : 'Enable memory'}
              onclick={() => void toggleMemory()}
              ><Power class="size-3" />{panel.state.RequestedEnabled
                ? 'Turn off'
                : 'Turn on'}</Button
            >
          </div>
          <div class="space-y-1.5">
            <Label for="memory-worker-model" class="text-xs">Worker model</Label><SelectField
              id="memory-worker-model"
              class="text-xs"
              bind:value={workerModel}
              placeholder="Choose a model"
              options={[
                ...(workerModel && !models.some((model) => model.ID === workerModel)
                  ? [{ value: workerModel, label: workerModel }]
                  : []),
                ...models.map((model) => ({ value: model.ID, label: modelLabel(model) })),
              ]}
              disabled={configurationDisabled}
            />
          </div>
          <p class="text-[10px] leading-5 text-muted-foreground">
            Used for context reduction and memory maintenance. A new model uses its default thinking
            setting. This changes only this workspace.
          </p>
          {#if instance.Stopped}<p class="text-xs text-muted-foreground">
              Resume this workspace to change settings.
            </p>{/if}
          <Button
            type="submit"
            class="h-10 w-full text-xs"
            disabled={configurationDisabled || !workerModel}
            >{#if panel.saving}<LoaderCircle class="size-3.5 animate-spin" />{/if}{enableAfterSave
              ? 'Save and enable memory'
              : 'Save worker model'}</Button
          >
        </form>
      {/if}
      {#if panel.loading && !panel.metrics}<p
          role="status"
          class="flex items-center gap-2 py-6 text-xs text-muted-foreground"
        >
          <LoaderCircle class="size-3.5 animate-spin" />Loading memory…
        </p>{/if}
      {#if panel.metrics}
        {#if panel.state && !panel.state.Enabled && !panel.state.Pending && panel.state.Available && !settingsOpen}<div
            class="border-b border-border py-3"
          >
            <p class="text-xs leading-5 text-muted-foreground">
              Memory is off. Stored records remain available when enabled.
            </p>
            <Button
              variant="ghost"
              class="mt-1 h-10 px-0 text-xs"
              disabled={configurationDisabled}
              onclick={() => void toggleMemory()}>Enable memory <Power class="size-3" /></Button
            >
          </div>{/if}
        <dl
          class="grid grid-cols-2 gap-x-3 gap-y-4 border-b border-border py-4"
          aria-label="Memory totals"
        >
          <div>
            <dt class="text-[11px] text-muted-foreground">Memories</dt>
            <dd class="mt-1 text-2xl font-medium tracking-tight tabular-nums">
              {memories.toLocaleString()}
            </dd>
            <p class="mt-0.5 text-[10px] text-muted-foreground">
              {retained.toLocaleString()} retained in history
            </p>
          </div>
          <div>
            <dt
              class="text-[11px] text-muted-foreground"
              title="Retained conversation messages and original notes"
            >
              Sources
            </dt>
            <dd class="mt-1 text-2xl font-medium tracking-tight tabular-nums">
              {count('source/').toLocaleString()}
            </dd>
            <p class="mt-0.5 text-[10px] text-muted-foreground">Messages and notes</p>
          </div>
          <div>
            <dt class="text-[11px] text-muted-foreground">Agent tokens</dt>
            <dd
              class="mt-1 text-xl font-medium tracking-tight tabular-nums"
              title={agentTokenCount(usage.statistics).toLocaleString()}
            >
              {formatTokenCount(agentTokenCount(usage.statistics))}
            </dd>
            <p class="mt-0.5 text-[10px] text-muted-foreground">
              {usage.statistics.Calls.toLocaleString()} model calls
            </p>
          </div>
          <div class="min-w-0">
            <dt class="text-[11px] text-muted-foreground">
              {usage.unpricedCalls && usage.statistics.Costs?.length
                ? 'Known agent cost'
                : costEstimated
                  ? 'Estimated agent cost'
                  : 'Agent cost'}
            </dt>
            <dd class="mt-1 break-words text-xl font-medium tracking-tight tabular-nums">
              {costLabel(usage.statistics)}
            </dd>
            <p class="mt-0.5 text-[10px] text-muted-foreground">Workspace total</p>
          </div>
        </dl>
        {#if usage.unpricedCalls}<p class="pt-3 text-[10px] leading-5 text-muted-foreground">
            {usage.unpricedCalls} calls have no token-price cost. This can include subscription usage.
          </p>{/if}
        <section class="border-b border-border py-4" aria-label="Memory jobs">
          <div class="mb-3 flex items-center gap-2">
            <Activity class="size-3.5 text-muted-foreground" />
            <h3 class="flex-1 text-xs font-medium">Jobs</h3>
            <span class="text-[10px] text-muted-foreground"
              >{count('job/running') ? 'Working' : count('job/pending') ? 'Queued' : 'Idle'}</span
            >
          </div>
          <dl class="grid grid-cols-3 gap-2 text-center text-xs tabular-nums">
            {#each [['Running', 'job/running'], ['Queued', 'job/pending'], ['Prepared', 'job/ready']] as [label, key]}<div
                class="rounded-md bg-muted/40 py-2"
              >
                <dd class="font-medium">{count(key).toLocaleString()}</dd>
                <dt class="mt-1 text-[10px] text-muted-foreground">{label}</dt>
              </div>{/each}
          </dl>
          {#if count('job/failed')}<p class="mt-2 text-[11px] text-destructive">
              {count('job/failed')} failed jobs
            </p>{/if}
        </section>
        <section class="border-b border-border py-4" aria-label="Memory agent usage">
          <h3 class="mb-1 text-xs font-medium">Agents</h3>
          {#each panel.metrics.Agents as agent (`${agent.Agent}/${agent.ModelID}`)}<MemoryAgentUsage
              {agent}
            />{:else}<p class="py-3 text-xs leading-5 text-muted-foreground">
              No agent calls yet. Direct memory saves do not use a language model.
            </p>{/each}
          <p class="mt-2 text-[10px] leading-5 text-muted-foreground">
            Agent usage belongs to this workspace, not the current chat.
          </p>
        </section>
        <section class="py-4" aria-label="Memory activity">
          <h3 class="mb-3 text-xs font-medium">Activity</h3>
          <dl class="space-y-2.5 text-[11px] tabular-nums">
            {#each [['Direct saves', 'context.remember/saved_memories'], ['Memory searches', 'context.main_search/queries'], ['Checkpoints', 'context.compactor/checkpoints'], ['Messages reduced', 'context.compactor/removed_messages'], ['Estimated tokens removed', 'context.compactor/estimated_tokens_removed']] as [label, key]}<div
                class="flex justify-between gap-3"
              >
                <dt class="text-muted-foreground">{label}</dt>
                <dd>{count(key).toLocaleString()}</dd>
              </div>{/each}
            <div class="flex justify-between gap-3">
              <dt class="text-muted-foreground">Embedding chunks</dt>
              <dd>{embeddingChunks.toLocaleString()}</dd>
            </div>
          </dl>
        </section>
      {/if}
    {/if}
  </div>
  <footer
    class="flex min-h-12 shrink-0 items-center gap-2 border-t border-border px-4 pb-[env(safe-area-inset-bottom)] text-[10px] text-muted-foreground"
  >
    <span class="min-w-0 flex-1"
      >{panel.updatedAt
        ? `Updated ${panel.updatedAt.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}`
        : 'Workspace memory'}{panel.metrics && !panel.error ? ' · Live' : ''}</span
    >
    <Button
      variant="ghost"
      size="icon"
      class="icon-button"
      aria-label="Refresh memory"
      title="Refresh memory"
      disabled={!api || !instance || panel.loading || panel.saving}
      onclick={() => void panel.refresh()}
      ><RefreshCw class={`size-3.5 ${panel.loading ? 'animate-spin' : ''}`} /></Button
    >
  </footer>
</section>
