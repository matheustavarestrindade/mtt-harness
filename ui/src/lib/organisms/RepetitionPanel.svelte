<script lang="ts">
  import { untrack } from 'svelte';
  import { LoaderCircle, Repeat2 } from 'lucide-svelte';
  import type { Instance, Model } from '../atoms/types';
  import type { RepetitionConfiguration } from '../atoms/plugins';
  import { Button } from '../atoms/ui/button';
  import { Input } from '../atoms/ui/input';
  import { Label } from '../atoms/ui/label';
  import { costLabel, formatTokenCount } from '../atoms/format';
  import { totalAgentUsage, agentTokenCount, durationLabel } from '../atoms/memory';
  import type { HarnessApi } from '../molecules/api/client';
  import SelectField from '../molecules/SelectField.svelte';
  import ModelCombobox from '../molecules/ModelCombobox.svelte';
  import ReasoningSelect from '../molecules/ReasoningSelect.svelte';
  import WorkerUsageBreakdown from '../molecules/WorkerUsageBreakdown.svelte';
  import MetricChart from '../molecules/charts/MetricChart.svelte';
  import { chartColors } from '../atoms/usage';
  import PluginPanelFooter from '../molecules/PluginPanelFooter.svelte';
  import SettingsPageTitle from '../molecules/SettingsPageTitle.svelte';
  import FieldHelp from '../molecules/FieldHelp.svelte';
  import PluginHeaderControls from '../molecules/PluginHeaderControls.svelte';
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
  const recoveries = $derived(
    [
      ['Running', 'recovery/running'],
      ['Completed', 'recovery/completed'],
      ['Failed', 'recovery/failed'],
      ['Cancelled', 'recovery/cancelled'],
      ['Interrupted', 'recovery/interrupted'],
    ].map(([label, key]) => ({ label, value: count(key) })),
  );
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

<section aria-label="Spaced repetition" class="min-w-0 w-full space-y-5">
  <SettingsPageTitle
    icon={Repeat2}
    title="Instructions"
    description="Instruction reminders and recovery."
    >{#snippet actions()}{#if panel.state}<PluginHeaderControls
          enabled={panel.state.RequestedEnabled}
          status={stateLabel}
          toggleLabel="Instruction reminders enabled"
          configureLabel="Configure reminders"
          {disabled}
          toggleDisabled={!panel.state.RequestedEnabled && !panel.state.Available}
          expanded={editing}
          onToggle={(enabled) => void panel.configure({ enabled })}
          onConfigure={openConfiguration}
        />{/if}{/snippet}</SettingsPageTitle
  >
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
            <Label for={`${identifier}-model`}>Recovery worker model</Label><ModelCombobox
              id={`${identifier}-model`}
              bind:value={workerModel}
              {disabled}
              {models}
              emptyLabel="Automatic reminders only"
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
      {#if count('reminders/low') || count('reminders/medium') || count('reminders/high')}<MetricChart
          title="Delivered reminders"
          description="All-time workspace deliveries by reminder level."
          data={[
            ['Low', 'reminders/low'],
            ['Medium', 'reminders/medium'],
            ['High', 'reminders/high'],
          ].map(([label, key]) => ({ label, value: count(key) }))}
          series={[{ key: 'value', label: 'Reminders', color: chartColors[0] }]}
          metric="calls"
          horizontal
          legend={false}
          height={144}
        />{:else}<p class="text-sm text-muted-foreground">No reminders delivered yet.</p>{/if}
      {#if count('reminders/deferred')}<p class="text-xs text-muted-foreground">
          {formatTokenCount(count('reminders/deferred'))} deferred reminders
        </p>{/if}
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
          <p
            class="text-xs text-muted-foreground"
            title="Uncached input + output. Cache is shown separately in worker usage."
          >
            Recovery worker tokens
          </p>
          <p class="mt-1 text-sm tabular-nums">
            {formatTokenCount(agentTokenCount(usage.statistics))}
          </p>
        </div>
        <div>
          <p class="flex items-center gap-1 text-xs text-muted-foreground">
            Recovery worker cost
            <FieldHelp
              label="About instruction recovery usage"
              text="Automatic reminders make no worker-model calls. Their input is already counted in session usage. Recovery costs cover calls with recorded prices."
            />
          </p>
          <p class="mt-1 text-sm tabular-nums">
            {estimatedCost ? '≈ ' : ''}{costLabel(usage.statistics)}
          </p>
        </div>
      </div>
      <div class="min-w-0 space-y-2">
        <MetricChart
          title="Recovery activity"
          description="Recorded recoveries by status."
          data={recoveries.some((recovery) => recovery.value > 0) ? recoveries : []}
          series={[{ key: 'value', label: 'Recoveries', color: chartColors[0] }]}
          metric="calls"
          horizontal
          categoryLabels="below"
          legend={false}
          height={180}
          emptyMessage="No recovery activity yet."
        />
        <p
          class="text-xs leading-5 text-muted-foreground tabular-nums"
          aria-label="Recovery context"
        >
          {count('recovery/queries').toLocaleString()} memory searches · {durationLabel(
            count('recovery/duration_ms'),
          )} recovery time
        </p>
      </div>
      {#if panel.metrics.Agents.length}<div>
          <h3 class="mb-2 text-sm font-medium">Recovery agents</h3>
          <WorkerUsageBreakdown agents={panel.metrics.Agents} {models} />
        </div>{/if}
    {/if}
    <PluginPanelFooter
      refreshLabel="Refresh Instructions"
      updatedAt={panel.updatedAt}
      refreshing={panel.loading}
      disabled={panel.saving}
      onRefresh={() => void panel.refresh()}
    />
  {/if}
</section>
