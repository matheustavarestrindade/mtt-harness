<script lang="ts">
  import { untrack } from 'svelte';
  import { LoaderCircle, ChartNoAxesCombined } from 'lucide-svelte';
  import { Label } from '../atoms/ui/label';
  import { Button } from '../atoms/ui/button';
  import * as Tabs from '../atoms/ui/tabs';
  import type { Instance, Session, Model } from '../atoms/types';
  import { costLabel, formatTokenCount, workspaceName } from '../atoms/format';
  import {
    aggregateUsageModels,
    chartColors,
    combineStatistics,
    modelProvider,
    recentUsageRange,
    usageMetrics,
    usageModelLabel,
    usageTimeline,
    usageValue,
    type UsageMetric,
    type UsageReport,
    type DateRange,
  } from '../atoms/usage';
  import type { HarnessApi } from '../molecules/api/client';
  import SelectField from '../molecules/SelectField.svelte';
  import MultiSelectField from '../molecules/MultiSelectField.svelte';
  import SettingsPageTitle from '../molecules/SettingsPageTitle.svelte';
  import PluginPanelFooter from '../molecules/PluginPanelFooter.svelte';
  import UsageDateRange from '../molecules/UsageDateRange.svelte';
  import MetricChart from '../molecules/charts/MetricChart.svelte';
  import Disclosure from '../molecules/Disclosure.svelte';

  let {
    api,
    instance,
    session,
    models = [],
  }: {
    api: HarnessApi;
    instance: Instance | null;
    session: Session | null;
    models?: Model[];
  } = $props();
  let scope = $state(''),
    range = $state<DateRange>(recentUsageRange(30)),
    provider = $state(''),
    modelIDs = $state<string[]>([]),
    metric = $state<UsageMetric>('tokens'),
    currency = $state('');
  let revision = $state(0),
    report = $state<UsageReport | null>(null),
    loading = $state(false),
    error = $state(''),
    updatedAt = $state<Date | null>(null),
    modelPage = $state(0);
  const identifier = $props.id();
  const scopes = $derived([
    { value: 'harness', label: 'Entire harness' },
    ...(instance
      ? [{ value: 'workspace', label: `Workspace · ${workspaceName(instance.Workspace)}` }]
      : []),
    ...(session ? [{ value: 'session', label: 'Current session' }] : []),
  ]);
  const selection = $derived(
    scopes.some((option) => option.value === scope) ? scope : instance ? 'workspace' : 'harness',
  );
  const scopeKey = $derived(
    `${selection}:${selection === 'workspace' ? instance?.ID : selection === 'session' ? session?.ID : ''}`,
  );
  let lastScope = '';
  $effect(() => {
    const key = scopeKey;
    untrack(() => {
      if (lastScope !== key) {
        lastScope = key;
        provider = '';
        modelIDs = [];
        currency = '';
        modelPage = 0;
      }
    });
  });
  $effect(() => {
    const currentAPI = api,
      from = range.from,
      to = range.to,
      instanceID = selection === 'workspace' ? instance?.ID : undefined,
      sessionID = selection === 'session' ? session?.ID : undefined;
    revision;
    const controller = new AbortController();
    loading = true;
    error = '';
    report = null;
    void currentAPI
      .usageReport(
        {
          from,
          to,
          ...(instanceID ? { instance_id: instanceID } : {}),
          ...(sessionID ? { session_id: sessionID } : {}),
        },
        controller.signal,
      )
      .then((value) => {
        if (!controller.signal.aborted) {
          report = value;
          updatedAt = new Date();
        }
      })
      .catch((failure) => {
        if (!controller.signal.aborted)
          error = failure instanceof Error ? failure.message : 'Cannot load usage.';
      })
      .finally(() => {
        if (!controller.signal.aborted) loading = false;
      });
    return () => controller.abort();
  });
  const allModels = $derived(
    [...new Set(report?.Buckets.map((bucket) => bucket.ModelID) ?? [])].sort(),
  );
  const providers = $derived([...new Set(allModels.map(modelProvider))].sort());
  const selectedBuckets = $derived(
    (report?.Buckets ?? []).filter(
      (bucket) =>
        (!provider || modelProvider(bucket.ModelID) === provider) &&
        (modelIDs.length === 0 || modelIDs.includes(bucket.ModelID)),
    ),
  );
  const totals = $derived(combineStatistics(selectedBuckets.map((bucket) => bucket.Statistics)));
  const pricedCalls = $derived(
    selectedBuckets.reduce((sum, bucket) => sum + bucket.PricedCalls, 0),
  );
  const failedCalls = $derived(
    selectedBuckets.reduce((sum, bucket) => sum + bucket.FailedCalls, 0),
  );
  const currencies = $derived(totals.Costs?.map((cost) => cost.Currency) ?? []);
  const selectedCurrency = $derived(
    currencies.includes(currency) ? currency : (currencies[0] ?? 'USD'),
  );
  const timeline = $derived.by(() => {
    const result = usageTimeline(selectedBuckets, range, models, metric, selectedCurrency);
    if (
      metric === 'cost' &&
      totals.Calls > 0 &&
      usageValue(totals, metric, selectedCurrency) === null
    )
      result.data = [];
    return result;
  });
  const rankedModels = $derived(
    aggregateUsageModels(selectedBuckets, models).sort(
      (first, second) =>
        (usageValue(second.statistics, metric, selectedCurrency) ?? -Infinity) -
          (usageValue(first.statistics, metric, selectedCurrency) ?? -Infinity) ||
        first.id.localeCompare(second.id),
    ),
  );
  const leadingModels = $derived(rankedModels.slice(0, 7));
  const restModels = $derived(rankedModels.slice(7));
  const comparison = $derived([
    ...leadingModels.map((row) => ({
      id: row.id,
      label: row.label,
      value: usageValue(row.statistics, metric, selectedCurrency),
    })),
    ...(restModels.length
      ? [
          {
            label: `Other models (${restModels.length})`,
            value: usageValue(
              combineStatistics(restModels.map((row) => row.statistics)),
              metric,
              selectedCurrency,
            ),
          },
        ]
      : []),
  ]);
  const description = $derived(
    metric === 'tokens'
      ? 'Uncached input + output. Reasoning is already included in output.'
      : metric === 'cache'
        ? 'Cache read + cache write, separate from input tokens.'
        : metric === 'cost'
          ? `${selectedCurrency} · recorded costs only${totals.Costs?.some((cost) => cost.Currency === selectedCurrency && cost.Estimated) ? ' · includes estimates' : ''}.`
          : 'Recorded provider requests, including calls without reported tokens.',
  );
  const pageModels = $derived(rankedModels.slice(modelPage * 25, (modelPage + 1) * 25));
  $effect(() => {
    scopeKey;
    range;
    provider;
    modelIDs;
    metric;
    currency;
    modelPage = 0;
  });
</script>

<section aria-label="Usage overview" class="min-w-0 w-full space-y-5">
  <SettingsPageTitle
    icon={ChartNoAxesCombined}
    title="Usage"
    description="Daily usage and model breakdowns from recorded requests."
  />
  <div class="grid min-w-0 grid-cols-2 gap-3">
    <div class="min-w-0 space-y-2">
      <Label for={`${identifier}-scope`}>Scope</Label><SelectField
        id={`${identifier}-scope`}
        value={selection}
        options={scopes}
        onValueChange={(value) => (scope = value)}
      />
    </div>
    <UsageDateRange value={range} onChange={(value) => (range = value)} />
    <div class="min-w-0 space-y-2">
      <Label for={`${identifier}-provider`}>Provider</Label><SelectField
        id={`${identifier}-provider`}
        value={provider}
        disabled={loading || !report}
        options={[
          { value: '', label: 'All providers' },
          ...providers.filter(Boolean).map((value) => ({ value, label: value })),
        ]}
        onValueChange={(value) => {
          provider = value;
          modelIDs = [];
        }}
      />
    </div>
    <div class="min-w-0 space-y-2">
      <Label for={`${identifier}-models`}>Models</Label><MultiSelectField
        id={`${identifier}-models`}
        value={modelIDs}
        disabled={loading || !report}
        placeholder="All models"
        options={[
          ...new Set([
            ...allModels.filter((id) => !provider || modelProvider(id) === provider),
            ...modelIDs,
          ]),
        ].map((id) => ({ value: id, label: usageModelLabel(id, models) }))}
        onValueChange={(value) => (modelIDs = value)}
      />
    </div>
  </div>
  {#if loading}<p role="status" class="flex items-center gap-2 py-8 text-sm text-muted-foreground">
      <LoaderCircle class="size-4 animate-spin" />Loading usage…
    </p>
  {:else if error}<p role="alert" class="break-words text-sm text-destructive">{error}</p>
  {:else if report}
    <dl
      class="grid min-w-0 grid-cols-3 gap-3 border-y border-border py-4"
      aria-label="Usage totals"
    >
      <div class="min-w-0">
        <dt class="text-xs text-muted-foreground">Tokens</dt>
        <dd
          class="mt-2 break-words text-xl font-medium tabular-nums"
          title={(totals.Input + totals.Output).toLocaleString()}
        >
          {formatTokenCount(totals.Input + totals.Output)}
        </dd>
      </div>
      <div class="min-w-0">
        <dt class="text-xs text-muted-foreground">Requests</dt>
        <dd class="mt-2 break-words text-xl font-medium tabular-nums">
          {totals.Calls.toLocaleString()}
        </dd>
      </div>
      <div class="min-w-0">
        <dt class="text-xs text-muted-foreground">Known cost</dt>
        <dd class="mt-2 break-words text-base font-medium tabular-nums">
          {totals.Costs?.some((cost) => cost.Estimated) ? '≈ ' : ''}{costLabel(totals)}
        </dd>
      </div>
    </dl>
    <Tabs.Root bind:value={metric} class="min-w-0 gap-4">
      <Tabs.List aria-label="Usage metric" class="grid min-h-[50px] w-full grid-cols-4"
        ><Tabs.Trigger value="tokens" class="min-h-11">Tokens</Tabs.Trigger><Tabs.Trigger
          value="cost"
          class="min-h-11">Cost</Tabs.Trigger
        ><Tabs.Trigger value="calls" class="min-h-11">Requests</Tabs.Trigger><Tabs.Trigger
          value="cache"
          class="min-h-11">Cache</Tabs.Trigger
        ></Tabs.List
      >
      {#each usageMetrics as option (option.value)}<Tabs.Content
          value={option.value}
          class="m-0 min-w-0 space-y-6"
          >{#if metric === option.value}
            {#if metric === 'cost' && currencies.length > 1}<div class="max-w-56 space-y-2">
                <Label for={`${identifier}-currency`}>Currency</Label><SelectField
                  id={`${identifier}-currency`}
                  value={selectedCurrency}
                  options={currencies.map((value) => ({ value, label: value }))}
                  onValueChange={(value) => (currency = value)}
                />
              </div>{/if}
            <MetricChart
              title={`Daily ${option.label.toLowerCase()}`}
              data={timeline.data}
              series={timeline.series}
              {metric}
              currency={selectedCurrency}
              dates
              description={`${description} Days use UTC.`}
            />
            <MetricChart
              title="By model"
              data={comparison}
              series={[{ key: 'value', label: option.label, color: chartColors[0] }]}
              {metric}
              currency={selectedCurrency}
              horizontal
              categoryLabels="below"
              height={Math.max(130, comparison.length * 36 + 32)}
              legend={false}
            />
          {/if}</Tabs.Content
        >{/each}
    </Tabs.Root>
    <Disclosure title="Usage details">
      <dl
        aria-label="Usage breakdown"
        class="mt-3 grid grid-cols-2 gap-x-4 gap-y-3 text-xs tabular-nums"
      >
        {#each [['Input', totals.Input], ['Output', totals.Output], ['Cache read', totals.CacheRead], ['Cache write', totals.CacheWrite], ['Reasoning (in output)', totals.Reasoning], ['Calls with cost', pricedCalls], ['Calls without cost', totals.Calls - pricedCalls], ['Failed calls', failedCalls]] as [label, value]}<div
          >
            <dt class="text-muted-foreground">{label}</dt>
            <dd class="mt-1">{Number(value).toLocaleString()}</dd>
          </div>{/each}
        <div>
          <dt class="text-muted-foreground">Cache hit</dt>
          <dd class="mt-1">{totals.CacheHitPercentage.toFixed(1)}%</dd>
        </div>
      </dl>
      <div class="mt-4 max-h-80 overflow-auto">
        <table class="w-full text-left text-xs tabular-nums">
          <caption class="sr-only">Usage by model</caption><thead
            ><tr
              >{#each ['Model', 'Input', 'Output', 'Cache read', 'Cache write', 'Requests', 'Cost'] as label}<th
                  class="sticky top-0 bg-background px-2 py-3">{label}</th
                >{/each}</tr
            ></thead
          ><tbody
            >{#each pageModels as row (row.id)}<tr class="border-t border-border"
                ><th class="max-w-56 break-words px-2 py-3 font-normal">{row.label}</th><td
                  class="p-2">{row.statistics.Input.toLocaleString()}</td
                ><td class="p-2">{row.statistics.Output.toLocaleString()}</td><td class="p-2"
                  >{row.statistics.CacheRead.toLocaleString()}</td
                ><td class="p-2">{row.statistics.CacheWrite.toLocaleString()}</td><td class="p-2"
                  >{row.statistics.Calls.toLocaleString()}</td
                ><td class="whitespace-nowrap p-2"
                  >{row.statistics.Costs?.some((cost) => cost.Estimated) ? '≈ ' : ''}{costLabel(
                    row.statistics,
                  )}</td
                ></tr
              >{/each}</tbody
          >
        </table>
      </div>
      {#if rankedModels.length > 25}<div class="mt-3 flex items-center justify-between gap-2">
          <Button
            variant="outline"
            class="h-11"
            disabled={modelPage === 0}
            onclick={() => modelPage--}>Previous</Button
          ><span class="text-xs">{modelPage + 1} / {Math.ceil(rankedModels.length / 25)}</span
          ><Button
            variant="outline"
            class="h-11"
            disabled={(modelPage + 1) * 25 >= rankedModels.length}
            onclick={() => modelPage++}>Next</Button
          >
        </div>{/if}
      <p class="mt-4 text-xs leading-5 text-muted-foreground">
        Input excludes cache. Output includes reasoning. Cost shows available prices in their
        original currencies; missing prices stay unknown, including subscription usage. {selection ===
        'session'
          ? 'Session totals include child agents.'
          : 'Workspace and harness totals include background workers.'}
      </p>
    </Disclosure>
  {/if}
  <PluginPanelFooter
    {updatedAt}
    refreshing={loading}
    disabled={loading}
    refreshLabel="Refresh usage"
    onRefresh={() => revision++}
  />
</section>
