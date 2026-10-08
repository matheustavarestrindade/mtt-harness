<script lang="ts">
  import { untrack } from 'svelte';
  import { Bot, Power, RefreshCw, Settings2 } from 'lucide-svelte';
  import type { Instance, Model } from '../atoms/types';
  import type { SidekickConfiguration } from '../atoms/plugins';
  import { Button } from '../atoms/ui/button';
  import { Input } from '../atoms/ui/input';
  import { Label } from '../atoms/ui/label';
  import { costLabel, formatTokenCount, modelLabel, workspaceName } from '../atoms/format';
  import { totalAgentUsage, agentTokenCount } from '../atoms/memory';
  import type { HarnessApi } from '../molecules/api/client';
  import SelectField from '../molecules/SelectField.svelte';
  import ReasoningSelect from '../molecules/ReasoningSelect.svelte';
  import MemoryAgentUsage from '../molecules/MemoryAgentUsage.svelte';
  import { WorkspacePluginPanelState } from './plugin-panel.svelte';

  let { api, instance }: { api: HarnessApi; instance: Instance | null } = $props();
  const panel = new WorkspacePluginPanelState<SidekickConfiguration>('sidekick', 'Sidekick');
  const identifier = $props.id();
  const workspaceID = $derived(instance?.ID ?? '');
  let models = $state<Model[]>([]);
  let modelsError = $state('');
  let editing = $state(false);
  let enableAfterSave = $state(false);
  let workerModel = $state('');
  let workerEffort = $state('');
  let memoryEnabled = $state(true);
  let filesEnabled = $state(true);
  let debounce = $state<number | undefined>(750);
  let cooldown = $state<number | undefined>(15000);
  let hintBytes = $state<number | undefined>(1600);
  let formError = $state('');
  const disabled = $derived(
    panel.saving || !!panel.state?.Pending || !panel.state || !!instance?.Stopped,
  );
  const selectedModel = $derived(models.find((model) => model.ID === workerModel));
  const usage = $derived(totalAgentUsage(panel.metrics?.Agents ?? []));
  const estimated = $derived(usage.statistics.Costs?.some((cost) => cost.Estimated));
  const count = (key: string) => panel.metrics?.Counters[key] ?? 0;

  $effect(() => {
    const client = api;
    const selected = workspaceID;
    untrack(() => panel.connect(client, selected));
    models = [];
    modelsError = '';
    editing = false;
    formError = '';
    const controller = new AbortController();
    if (selected)
      void client
        .models(selected, controller.signal)
        .then((result) => {
          if (!controller.signal.aborted) models = result;
        })
        .catch((failure) => {
          if (!controller.signal.aborted)
            modelsError = failure instanceof Error ? failure.message : 'Cannot load worker models.';
        });
    const visibility = () => (document.hidden ? panel.pause() : void panel.refresh());
    document.addEventListener('visibilitychange', visibility);
    return () => {
      controller.abort();
      panel.disconnect();
      document.removeEventListener('visibilitychange', visibility);
    };
  });

  function openConfiguration(enable = false) {
    const value = panel.state?.Configuration;
    if (!value) return;
    workerModel = value.worker_model;
    workerEffort = value.worker_effort;
    memoryEnabled = value.memory_enabled;
    filesEnabled = value.files_enabled;
    debounce = value.debounce_ms;
    cooldown = value.cooldown_ms;
    hintBytes = value.hint_bytes;
    enableAfterSave = enable;
    formError = '';
    panel.actionError = '';
    editing = true;
  }
  async function toggleSidekick() {
    if (!panel.state) return;
    if (!panel.state.RequestedEnabled && !panel.state.Configuration.worker_model) {
      openConfiguration(true);
      return;
    }
    await panel.configure({ enabled: !panel.state.RequestedEnabled });
  }
  async function saveConfiguration() {
    if (!workerModel) {
      formError = 'Choose a worker model.';
      return;
    }
    if (!memoryEnabled && !filesEnabled) {
      formError = 'Enable at least one retrieval source.';
      return;
    }
    if (
      !Number.isInteger(debounce) ||
      debounce === undefined ||
      debounce < 0 ||
      debounce > 10000 ||
      !Number.isInteger(cooldown) ||
      cooldown === undefined ||
      cooldown < 0 ||
      cooldown > 300000 ||
      !Number.isInteger(hintBytes) ||
      hintBytes === undefined ||
      hintBytes < 128 ||
      hintBytes > 4096
    ) {
      formError = 'Use valid delays and a note budget from 128 to 4,096 bytes.';
      return;
    }
    formError = '';
    if (
      await panel.configure({
        worker_model: workerModel,
        worker_effort: workerEffort,
        memory_enabled: memoryEnabled,
        files_enabled: filesEnabled,
        debounce_ms: debounce,
        cooldown_ms: cooldown,
        hint_bytes: hintBytes,
        ...(enableAfterSave ? { enabled: true } : {}),
      })
    )
      editing = false;
  }
</script>

<section aria-label="Sidekick settings" class="min-w-0 space-y-5">
  <div class="flex items-start gap-3">
    <Bot class="mt-1 size-5 shrink-0 text-muted-foreground" />
    <div class="min-w-0 flex-1">
      <h2 class="text-lg font-medium">Sidekick</h2>
      <p class="mt-1 text-sm text-muted-foreground">
        Useful workspace context for the agent’s current DOING activity.
      </p>
    </div>
    {#if panel.state}<span role="status" class="text-xs text-muted-foreground"
        >{panel.state.Pending
          ? 'Updating…'
          : !panel.state.Available
            ? 'Unavailable'
            : panel.state.Enabled
              ? 'On'
              : 'Off'}</span
      >{/if}
  </div>
  {#if !instance}<p class="text-sm text-muted-foreground">
      Select a workspace to configure Sidekick.
    </p>
  {:else}
    <p class="break-words text-xs text-muted-foreground" title={instance.Workspace}>
      {workspaceName(instance.Workspace)} · Workspace settings
    </p>
    {#if panel.error}<p role="alert" class="break-words text-sm text-destructive">
        {panel.error}
      </p>{/if}
    {#if panel.state?.Error}<p role="status" class="break-words text-xs text-muted-foreground">
        {panel.state.Error}
      </p>{/if}
    {#if panel.state}
      <div class="flex flex-wrap gap-2">
        <Button
          variant={panel.state.RequestedEnabled ? 'secondary' : 'default'}
          class="h-11"
          disabled={disabled || (!panel.state.Available && !panel.state.RequestedEnabled)}
          onclick={toggleSidekick}
          ><Power class="size-4" />{panel.state.RequestedEnabled
            ? 'Disable Sidekick'
            : 'Enable Sidekick'}</Button
        >
        <Button variant="outline" class="h-11" {disabled} onclick={() => openConfiguration()}
          ><Settings2 class="size-4" />Configure Sidekick</Button
        >
      </div>
    {/if}
    {#if editing}
      <form
        class="space-y-4 border-y border-border py-5"
        onsubmit={(event) => {
          event.preventDefault();
          void saveConfiguration();
        }}
      >
        <div class="space-y-2">
          <Label for={`${identifier}-model`}>Worker model</Label><SelectField
            id={`${identifier}-model`}
            value={workerModel}
            onValueChange={(value) => {
              if (value !== workerModel) workerEffort = '';
              workerModel = value;
            }}
            {disabled}
            placeholder="Choose a worker model"
            options={[
              ...(workerModel && !models.some((model) => model.ID === workerModel)
                ? [{ value: workerModel, label: workerModel }]
                : []),
              ...models.map((model) => ({ value: model.ID, label: modelLabel(model) })),
            ]}
          />
        </div>
        {#if modelsError}<p role="alert" class="text-sm text-destructive">{modelsError}</p>{/if}
        <ReasoningSelect
          model={selectedModel}
          value={workerEffort}
          onChange={(value) => (workerEffort = value)}
          stacked
          {disabled}
        />
        <div class="space-y-2">
          <p class="text-xs font-medium">Retrieval sources</p>
          <div class="flex flex-wrap gap-2">
            <Button
              variant={memoryEnabled ? 'secondary' : 'outline'}
              class="h-11"
              aria-pressed={memoryEnabled}
              {disabled}
              onclick={() => (memoryEnabled = !memoryEnabled)}>Workspace memory</Button
            ><Button
              variant={filesEnabled ? 'secondary' : 'outline'}
              class="h-11"
              aria-pressed={filesEnabled}
              {disabled}
              onclick={() => (filesEnabled = !filesEnabled)}>Project files</Button
            >
          </div>
          <p class="text-xs text-muted-foreground">
            Medium-compression memory and bounded, read-only file excerpts. No shell commands or
            file changes.
          </p>
        </div>
        <div class="grid gap-4 sm:grid-cols-2">
          <div class="space-y-2">
            <Label for={`${identifier}-debounce`}>Debounce (ms)</Label><Input
              id={`${identifier}-debounce`}
              type="number"
              min="0"
              max="10000"
              step="1"
              bind:value={debounce}
              {disabled}
            />
          </div>
          <div class="space-y-2">
            <Label for={`${identifier}-cooldown`}>Cooldown (ms)</Label><Input
              id={`${identifier}-cooldown`}
              type="number"
              min="0"
              max="300000"
              step="1"
              bind:value={cooldown}
              {disabled}
            />
          </div>
        </div>
        <div class="space-y-2">
          <Label for={`${identifier}-bytes`}>Note budget (UTF-8 bytes)</Label><Input
            id={`${identifier}-bytes`}
            type="number"
            min="128"
            max="4096"
            step="1"
            bind:value={hintBytes}
            {disabled}
          />
        </div>
        {#if formError || panel.actionError}<p
            role="alert"
            class="break-words text-sm text-destructive"
          >
            {formError || panel.actionError}
          </p>{/if}
        <div class="flex flex-wrap gap-2">
          <Button type="submit" class="h-11" {disabled}
            >{panel.saving
              ? 'Saving…'
              : enableAfterSave
                ? 'Save and enable Sidekick'
                : 'Save Sidekick settings'}</Button
          ><Button
            variant="ghost"
            class="h-11"
            disabled={panel.saving}
            onclick={() => (editing = false)}>Cancel</Button
          >
        </div>
      </form>
    {:else if panel.actionError}<p role="alert" class="break-words text-sm text-destructive">
        {panel.actionError}
      </p>{/if}
    {#if panel.metrics}
      <dl class="grid grid-cols-2 gap-5 border-y border-border py-5">
        <div>
          <dt class="text-xs text-muted-foreground">Notes delivered</dt>
          <dd class="mt-1 text-xl tabular-nums">{formatTokenCount(count('hints/delivered'))}</dd>
        </div>
        <div>
          <dt class="text-xs text-muted-foreground">Active work</dt>
          <dd class="mt-1 text-xl tabular-nums">{formatTokenCount(count('jobs/active'))}</dd>
        </div>
        <div>
          <dt class="text-xs text-muted-foreground">Worker tokens</dt>
          <dd class="mt-1 text-lg tabular-nums">
            {formatTokenCount(agentTokenCount(usage.statistics))}
          </dd>
        </div>
        <div>
          <dt class="text-xs text-muted-foreground">Worker cost</dt>
          <dd class="mt-1 break-words text-lg tabular-nums">
            {estimated ? '≈ ' : ''}{costLabel(usage.statistics)}
          </dd>
        </div>
      </dl>
      <dl class="grid grid-cols-2 gap-x-4 gap-y-2 text-xs">
        <dt class="text-muted-foreground">Memory results</dt>
        <dd class="text-right tabular-nums">{formatTokenCount(count('sources/memories'))}</dd>
        <dt class="text-muted-foreground">File excerpts</dt>
        <dd class="text-right tabular-nums">{formatTokenCount(count('sources/files'))}</dd>
        <dt class="text-muted-foreground">Duplicate sources skipped</dt>
        <dd class="text-right tabular-nums">{formatTokenCount(count('sources/duplicates'))}</dd>
        <dt class="text-muted-foreground">Already in context</dt>
        <dd class="text-right tabular-nums">
          {formatTokenCount(count('sources/already_present'))}
        </dd>
        <dt class="text-muted-foreground">Runs with no useful note</dt>
        <dd class="text-right tabular-nums">{formatTokenCount(count('jobs/skipped'))}</dd>
        <dt class="text-muted-foreground">Stale results discarded</dt>
        <dd class="text-right tabular-nums">
          {formatTokenCount(count('jobs/stale') + count('hints/stale'))}
        </dd>
        <dt class="text-muted-foreground">Cancelled runs</dt>
        <dd class="text-right tabular-nums">{formatTokenCount(count('jobs/cancelled'))}</dd>
        <dt class="text-muted-foreground">Added request input</dt>
        <dd class="text-right tabular-nums">
          {count('hints/estimated_tokens') ? '≈ ' : ''}{formatTokenCount(
            count('hints/input_tokens'),
          )}
        </dd>
      </dl>
      <p class="text-xs text-muted-foreground">
        Worker usage belongs to this workspace. Added hint input is already included in chat usage.
      </p>
      {#if panel.metrics.Agents.length}<div>
          <h3 class="mb-2 text-sm font-medium">Agent usage</h3>
          {#each panel.metrics.Agents as agent (`${agent.Agent}:${agent.ModelID}`)}<MemoryAgentUsage
              {agent}
            />{/each}
        </div>{/if}
    {:else if panel.loading}<p role="status" class="text-sm text-muted-foreground">
        Loading Sidekick…
      </p>{/if}
    <div class="flex items-center justify-between gap-3 border-t border-border pt-3">
      <span class="text-xs text-muted-foreground"
        >{panel.updatedAt
          ? `Updated ${panel.updatedAt.toLocaleTimeString()}`
          : 'Workspace activity'}</span
      ><Button
        variant="ghost"
        class="h-11"
        disabled={panel.loading || panel.saving}
        onclick={() => void panel.refresh()}
        ><RefreshCw class={`size-4 ${panel.loading ? 'animate-spin' : ''}`} />Refresh Sidekick</Button
      >
    </div>
  {/if}
</section>
