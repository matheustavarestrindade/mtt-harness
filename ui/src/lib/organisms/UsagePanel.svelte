<script lang="ts">
  import { LoaderCircle, RefreshCw } from 'lucide-svelte';
  import { Button } from '../atoms/ui/button';
  import { Label } from '../atoms/ui/label';
  import type { Instance, Session, Statistics } from '../atoms/types';
  import { costLabel, workspaceName } from '../atoms/format';
  import type { HarnessApi } from '../molecules/api/client';
  import SelectField from '../molecules/SelectField.svelte';

  let {
    api,
    instance,
    session,
  }: { api: HarnessApi; instance: Instance | null; session: Session | null } = $props();
  let scope = $state('');
  let revision = $state(0);
  let statistics = $state<Statistics | null>(null);
  let loading = $state(false);
  let error = $state('');
  const integer = (value: number) => new Intl.NumberFormat().format(value);
  const selection = $derived(scope || (session ? 'session' : instance ? 'workspace' : 'harness'));
  const metrics = $derived(
    statistics
      ? [
          {
            label: 'Input tokens',
            value: integer(statistics.Input + statistics.CacheRead + statistics.CacheWrite),
          },
          { label: 'Output tokens', value: integer(statistics.Output) },
          { label: 'Model calls', value: integer(statistics.Calls) },
          { label: 'Cache hit', value: `${statistics.CacheHitPercentage.toFixed(1)}%` },
          { label: 'Reasoning tokens', value: integer(statistics.Reasoning) },
          {
            label:
              statistics.Cost?.Estimated || statistics.Costs?.some((cost) => cost.Estimated)
                ? 'Estimated cost'
                : 'Recorded cost',
            value: costLabel(statistics),
          },
        ]
      : [],
  );

  $effect(() => {
    const currentAPI = api;
    const currentScope = selection;
    const instanceID = instance?.ID;
    const sessionID = session?.ID;
    revision;
    const controller = new AbortController();
    loading = true;
    error = '';
    statistics = null;
    const request =
      currentScope === 'session' && sessionID
        ? currentAPI.statistics(sessionID, controller.signal)
        : currentScope === 'workspace' && instanceID
          ? currentAPI.instanceStatistics(instanceID, controller.signal)
          : currentAPI.harnessStatistics(controller.signal);
    void request
      .then((value) => {
        if (!controller.signal.aborted) statistics = value;
      })
      .catch((failure: unknown) => {
        if (!controller.signal.aborted)
          error = failure instanceof Error ? failure.message : 'Cannot load usage.';
      })
      .finally(() => {
        if (!controller.signal.aborted) loading = false;
      });
    return () => controller.abort();
  });
</script>

<section aria-label="Usage overview" class="mx-auto w-full max-w-2xl">
  <div class="mb-6 flex items-start justify-between gap-3">
    <div>
      <h2 class="text-xl font-medium tracking-tight">Usage</h2>
      <p class="mt-2 text-sm leading-6 text-muted-foreground">
        Recorded model usage, including child agents. Totals cover the available history.
      </p>
    </div>
    <Button
      variant="ghost"
      size="icon"
      class="icon-button shrink-0"
      aria-label="Refresh usage"
      disabled={loading}
      onclick={() => revision++}><RefreshCw class="size-4" /></Button
    >
  </div>
  <div class="mb-6 space-y-2">
    <Label for="usage-scope">Usage scope</Label>
    <SelectField
      id="usage-scope"
      value={selection}
      options={[
        { value: 'harness', label: 'Entire harness' },
        ...(instance
          ? [{ value: 'workspace', label: `Workspace · ${workspaceName(instance.Workspace)}` }]
          : []),
        ...(session ? [{ value: 'session', label: 'Current session' }] : []),
      ]}
      onValueChange={(value) => (scope = value)}
    />
  </div>
  {#if loading}<p role="status" class="flex items-center gap-2 text-sm text-muted-foreground">
      <LoaderCircle class="size-4 animate-spin" />Loading usage…
    </p>
  {:else if error}<p role="alert" class="break-words text-sm text-destructive">{error}</p>
  {:else if statistics}
    <dl class="grid min-w-0 grid-cols-2 gap-3">
      {#each metrics as metric (metric.label)}
        <div class="min-w-0 rounded-xl border border-border bg-card/50 p-4 sm:p-5">
          <dt class="text-xs text-muted-foreground">{metric.label}</dt>
          <dd class="mt-3 break-words text-xl font-medium tracking-tight sm:text-2xl">
            {metric.value}
          </dd>
        </div>
      {/each}
    </dl>
    <dl class="mt-6 space-y-3 border-t border-border pt-5 text-sm">
      <div class="flex justify-between gap-3">
        <dt class="text-muted-foreground">Uncached input</dt>
        <dd>{integer(statistics.Input)}</dd>
      </div>
      <div class="flex justify-between gap-3">
        <dt class="text-muted-foreground">Cache read</dt>
        <dd>{integer(statistics.CacheRead)}</dd>
      </div>
      <div class="flex justify-between gap-3">
        <dt class="text-muted-foreground">Cache write</dt>
        <dd>{integer(statistics.CacheWrite)}</dd>
      </div>
    </dl>
    <p class="mt-5 text-xs leading-5 text-muted-foreground">
      Input includes cached tokens. Reasoning tokens are shown separately and are not added to
      output. Costs include only calls with available pricing. Catalog estimates can differ from
      provider bills, including discounts and time-based rates. Subscription usage has no per-token
      charge estimate.
    </p>
  {/if}
</section>
