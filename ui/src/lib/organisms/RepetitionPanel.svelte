<script lang="ts">
  import { untrack } from 'svelte';
  import { LoaderCircle, RefreshCw, Repeat2, Power, Settings2 } from 'lucide-svelte';
  import type { Instance, Model } from '../atoms/types';
  import type { RepetitionConfiguration } from '../atoms/plugins';
  import { Button } from '../atoms/ui/button';
  import { Input } from '../atoms/ui/input';
  import { Label } from '../atoms/ui/label';
  import { costLabel, formatTokenCount, modelLabel, workspaceName } from '../atoms/format';
  import { totalAgentUsage, agentTokenCount, durationLabel } from '../atoms/memory';
  import type { HarnessApi } from '../molecules/api/client';
  import SelectField from '../molecules/SelectField.svelte';
  import ReasoningSelect from '../molecules/ReasoningSelect.svelte';
  import MemoryAgentUsage from '../molecules/MemoryAgentUsage.svelte';
  import { WorkspacePluginPanelState } from './plugin-panel.svelte';

  let { api, instance }: { api: HarnessApi; instance: Instance | null } = $props();
  const panel = new WorkspacePluginPanelState<RepetitionConfiguration>(
    'spaced_repetition',
    'Spaced repetition',
  );
  const identifier = $props.id();
  const workspaceID = $derived(instance?.ID ?? '');
  let models = $state<Model[]>([]);
  let modelsError = $state('');
  let editing = $state(false);
  let mode = $state('model_fraction');
  let tokens = $state<number | undefined>(32768);
  let percentage = $state<number | undefined>(12.5);
  let maximum = $state<number | undefined>(32768);
  let pattern = $state('low, low, low, medium');
  let workerModel = $state('');
  let workerEffort = $state('');
  let formError = $state('');
  const disabled = $derived(
    panel.saving || !!panel.state?.Pending || !panel.state || !!instance?.Stopped,
  );
  const count = (name: string) => panel.metrics?.Counters[name] ?? 0;
  const usage = $derived(totalAgentUsage(panel.metrics?.Agents ?? []));
  const estimatedCost = $derived(usage.statistics.Costs?.some((cost) => cost.Estimated));
  const selectedModel = $derived(models.find((model) => model.ID === workerModel));
  const stateLabel = $derived(
    panel.state?.Pending
      ? 'Updating…'
      : !panel.state?.Available
        ? 'Unavailable'
        : panel.state.Enabled
          ? 'On'
          : 'Off',
  );

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

  function openConfiguration() {
    const value = panel.state?.Configuration;
    if (!value) return;
    mode = value.interval.mode;
    tokens = value.interval.tokens;
    percentage = value.interval.fraction * 100;
    maximum = value.interval.max_tokens;
    pattern = value.pattern.join(', ');
    workerModel = value.worker_model;
    workerEffort = value.worker_effort;
    formError = '';
    panel.actionError = '';
    editing = true;
  }
  async function saveConfiguration() {
    const levels = pattern.split(',').map((level) => level.trim().toLowerCase());
    if (
      !levels.length ||
      levels.length > 32 ||
      levels.some((level) => level !== 'low' && level !== 'medium')
    ) {
      formError = 'Use 1–32 low or medium levels, separated by commas.';
      return;
    }
    if (
      !Number.isInteger(tokens) ||
      !Number.isInteger(maximum) ||
      !tokens ||
      !maximum ||
      tokens < 256 ||
      maximum < 256 ||
      tokens > 2097152 ||
      maximum > 2097152 ||
      percentage === undefined ||
      !Number.isFinite(percentage) ||
      percentage < 1 ||
      percentage > 50
    ) {
      formError = 'Use token values from 256 to 2,097,152 and a context percentage from 1 to 50.';
      return;
    }
    formError = '';
    if (
      await panel.configure({
        interval: {
          mode: mode as 'tokens' | 'model_fraction',
          tokens,
          fraction: percentage / 100,
          max_tokens: maximum,
        },
        pattern: levels as ('low' | 'medium')[],
        worker_model: workerModel,
        worker_effort: workerEffort,
      })
    )
      editing = false;
  }
</script>

<section aria-label="Spaced repetition" class="mx-auto min-w-0 max-w-2xl space-y-6">
  <header class="flex min-w-0 flex-wrap items-start justify-between gap-3">
    <div class="min-w-0">
      <h2 class="flex items-center gap-2 text-xl font-medium">
        <Repeat2 class="size-5 shrink-0" />Spaced repetition
      </h2>
      <p class="mt-2 text-sm text-muted-foreground">
        Instruction reminders and recovery for {instance
          ? workspaceName(instance.Workspace)
          : 'the selected workspace'}.
      </p>
    </div>
    {#if workspaceID}<Button
        variant="ghost"
        size="icon"
        class="icon-button"
        aria-label="Refresh instruction usage"
        disabled={panel.loading || panel.saving}
        onclick={() => panel.refresh()}
        ><RefreshCw class={panel.loading ? 'size-4 animate-spin' : 'size-4'} /></Button
      >{/if}
  </header>
  {#if !instance}
    <p class="text-sm text-muted-foreground">
      Select a workspace to configure instruction reminders.
    </p>
  {:else}
    {#if panel.error}<p
        role="alert"
        class="rounded-md border border-border bg-muted/30 p-3 text-sm"
      >
        {panel.error}
      </p>{/if}
    {#if panel.state}
      <div class="flex flex-wrap items-center justify-between gap-3 border-b border-border pb-4">
        <div>
          <p class="text-sm font-medium" role="status">{stateLabel}</p>
          <p class="mt-1 text-xs text-muted-foreground">
            Changes apply after the active tool group.
          </p>
        </div>
        <div class="flex flex-wrap gap-2">
          <Button
            variant="outline"
            class="h-11 gap-2"
            disabled={disabled || (!panel.state.RequestedEnabled && !panel.state.Available)}
            aria-label={panel.state.RequestedEnabled
              ? 'Disable instruction reminders'
              : 'Enable instruction reminders'}
            onclick={() => panel.configure({ enabled: !panel.state!.RequestedEnabled })}
            ><Power class="size-4" />{panel.state.RequestedEnabled ? 'Turn off' : 'Turn on'}</Button
          >
          <Button variant="outline" class="h-11 gap-2" {disabled} onclick={openConfiguration}
            ><Settings2 class="size-4" />Configure reminders</Button
          >
        </div>
      </div>
      {#if panel.state.Error}<p class="text-sm text-muted-foreground">{panel.state.Error}</p>{/if}
      {#if editing}
        <form
          class="space-y-4 border-b border-border pb-5"
          onsubmit={(event) => {
            event.preventDefault();
            void saveConfiguration();
          }}
        >
          <div class="space-y-2">
            <Label for={`${identifier}-mode`}>Growth interval</Label><SelectField
              id={`${identifier}-mode`}
              bind:value={mode}
              {disabled}
              options={[
                { value: 'model_fraction', label: 'Model context percentage' },
                { value: 'tokens', label: 'Fixed token interval' },
              ]}
            />
          </div>
          {#if mode === 'model_fraction'}
            <div class="grid min-w-0 grid-cols-1 gap-4 sm:grid-cols-2">
              <div class="space-y-2">
                <Label for={`${identifier}-percentage`}>Context percentage</Label><Input
                  id={`${identifier}-percentage`}
                  type="number"
                  min="1"
                  max="50"
                  step="0.5"
                  bind:value={percentage}
                  {disabled}
                />
              </div>
              <div class="space-y-2">
                <Label for={`${identifier}-maximum`}>Maximum interval tokens</Label><Input
                  id={`${identifier}-maximum`}
                  type="number"
                  min="256"
                  max="2097152"
                  bind:value={maximum}
                  {disabled}
                />
              </div>
            </div>
          {:else}<div class="space-y-2">
              <Label for={`${identifier}-tokens`}>Interval tokens</Label><Input
                id={`${identifier}-tokens`}
                type="number"
                min="256"
                max="2097152"
                bind:value={tokens}
                {disabled}
              />
            </div>{/if}
          <div class="space-y-2">
            <Label for={`${identifier}-pattern`}>Reminder pattern</Label><Input
              id={`${identifier}-pattern`}
              bind:value={pattern}
              {disabled}
              aria-describedby={`${identifier}-pattern-help`}
            />
            <p id={`${identifier}-pattern-help`} class="text-xs text-muted-foreground">
              Comma-separated low and medium levels. The pattern repeats; high recovery is on
              demand.
            </p>
          </div>
          <div class="space-y-2">
            <Label for={`${identifier}-model`}>Recovery worker model</Label><SelectField
              id={`${identifier}-model`}
              bind:value={workerModel}
              {disabled}
              options={[
                { value: '', label: 'Automatic reminders only' },
                ...models.map((model) => ({ value: model.ID, label: modelLabel(model) })),
                ...(workerModel && !models.some((model) => model.ID === workerModel)
                  ? [{ value: workerModel, label: `Unavailable: ${workerModel}`, disabled: true }]
                  : []),
              ]}
              onValueChange={(value) => {
                workerModel = value;
                workerEffort = '';
              }}
            />
            <p class="text-xs text-muted-foreground">
              Choose a model to enable remember_instructions. Worker usage belongs to this
              workspace.
            </p>
          </div>
          <ReasoningSelect
            model={selectedModel}
            value={workerEffort}
            {disabled}
            stacked
            onChange={(effort) => (workerEffort = effort)}
          />
          {#if modelsError}<p role="alert" class="text-sm text-destructive">{modelsError}</p>{/if}
          {#if formError}<p role="alert" class="text-sm text-destructive">{formError}</p>{/if}
          <div class="flex flex-wrap gap-2">
            <Button type="submit" class="h-11 gap-2" {disabled}
              >{#if panel.saving}<LoaderCircle class="size-4 animate-spin" />{/if}Save reminder
              settings</Button
            ><Button
              type="button"
              variant="ghost"
              class="h-11"
              onclick={() => (editing = false)}
              disabled={panel.saving}>Cancel</Button
            >
          </div>
        </form>
      {:else}
        <div class="space-y-2 text-sm">
          <p>
            {panel.state.Configuration.interval.mode === 'tokens'
              ? `Every ${formatTokenCount(panel.state.Configuration.interval.tokens)} input tokens of growth`
              : `${panel.state.Configuration.interval.fraction * 100}% of model context, capped at ${formatTokenCount(panel.state.Configuration.interval.max_tokens)} tokens`}
          </p>
          <p class="break-words text-xs text-muted-foreground">
            {panel.state.Configuration.pattern.join(' → ')} → repeat
          </p>
          <p class="break-words text-xs text-muted-foreground">
            Recovery: {panel.state.Configuration.worker_model || 'No worker model selected'}
          </p>
        </div>
      {/if}
    {:else if panel.loading}<p role="status" class="text-sm text-muted-foreground">
        Loading instruction reminders…
      </p>{/if}
    {#if panel.actionError}<p role="alert" class="text-sm text-destructive">
        {panel.actionError}
      </p>{/if}
    {#if panel.metrics}
      <dl class="grid grid-cols-2 gap-5 sm:grid-cols-4">
        {#each [{ label: 'Low reminders', key: 'reminders/low' }, { label: 'Medium reminders', key: 'reminders/medium' }, { label: 'High recoveries', key: 'reminders/high' }, { label: 'Deferred reminders', key: 'reminders/deferred' }] as item}<div
          >
            <dt class="text-xs text-muted-foreground">{item.label}</dt>
            <dd class="mt-1 text-xl font-medium tabular-nums">
              {formatTokenCount(count(item.key))}
            </dd>
          </div>{/each}
      </dl>
      <div class="grid grid-cols-1 gap-4 border-y border-border py-4 sm:grid-cols-3">
        <div>
          <p class="text-xs text-muted-foreground">Reminder input added</p>
          <p class="mt-1 text-sm tabular-nums">
            {count('reminders/estimated_tokens') > 0 ? '≈ ' : ''}{formatTokenCount(
              count('reminders/tokens'),
            )} tokens
          </p>
        </div>
        <div>
          <p class="text-xs text-muted-foreground">Recovery worker tokens</p>
          <p class="mt-1 text-sm tabular-nums">
            {formatTokenCount(agentTokenCount(usage.statistics))}
          </p>
        </div>
        <div>
          <p class="text-xs text-muted-foreground">Recovery worker cost</p>
          <p class="mt-1 text-sm tabular-nums">
            {estimatedCost ? '≈ ' : ''}{costLabel(usage.statistics)}
          </p>
        </div>
      </div>
      <p class="text-xs leading-5 text-muted-foreground">
        Automatic reminders make no worker-model calls. Their input is already counted in session
        usage. Recovery costs cover calls with recorded prices.
      </p>
      <dl class="grid grid-cols-2 gap-x-6 gap-y-3 text-xs">
        <div>
          <dt class="text-muted-foreground">Running recoveries</dt>
          <dd class="mt-1 tabular-nums">{count('recovery/running')}</dd>
        </div>
        <div>
          <dt class="text-muted-foreground">Completed recoveries</dt>
          <dd class="mt-1 tabular-nums">{count('recovery/completed')}</dd>
        </div>
        <div>
          <dt class="text-muted-foreground">Failed / cancelled / interrupted</dt>
          <dd class="mt-1 tabular-nums">
            {count('recovery/failed')} / {count('recovery/cancelled')} / {count(
              'recovery/interrupted',
            )}
          </dd>
        </div>
        <div>
          <dt class="text-muted-foreground">Memory searches</dt>
          <dd class="mt-1 tabular-nums">{count('recovery/queries')}</dd>
        </div>
        <div>
          <dt class="text-muted-foreground">Recovery time</dt>
          <dd class="mt-1 tabular-nums">{durationLabel(count('recovery/duration_ms'))}</dd>
        </div>
      </dl>
      {#if panel.metrics.Agents.length}<div>
          <h3 class="mb-2 text-sm font-medium">Recovery agents</h3>
          {#each panel.metrics.Agents as agent (`${agent.Agent}/${agent.ModelID}`)}<MemoryAgentUsage
              {agent}
            />{/each}
        </div>{/if}
    {/if}
    {#if panel.updatedAt}<p class="text-xs text-muted-foreground">
        Last update {panel.updatedAt.toLocaleTimeString()}. Workspace totals across sessions.
      </p>{/if}
  {/if}
</section>
