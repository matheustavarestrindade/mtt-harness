<script lang="ts">
  import type { MemoryAgentUsage as AgentUsage } from '../atoms/memory';
  import { memoryAgentLabel } from '../atoms/memory';
  import type { Model, Statistics } from '../atoms/types';
  import {
    chartColors,
    combineStatistics,
    usageMetrics,
    usageModelLabel,
    usageValue,
    type UsageMetric,
  } from '../atoms/usage';
  import { Label } from '../atoms/ui/label';
  import { Button } from '../atoms/ui/button';
  import SelectField from './SelectField.svelte';
  import MetricChart from './charts/MetricChart.svelte';
  import Disclosure from './Disclosure.svelte';
  import MemoryAgentUsage from './MemoryAgentUsage.svelte';
  let {
    agents,
    models = [],
    agentLabel = memoryAgentLabel,
  }: { agents: AgentUsage[]; models?: Model[]; agentLabel?: (name: string) => string } = $props();
  const identifier = $props.id();
  let metric = $state<UsageMetric>('tokens'),
    group = $state('model'),
    currency = $state(''),
    page = $state(0);
  const totals = $derived(combineStatistics(agents.map((agent) => agent.Statistics)));
  const currencies = $derived(totals.Costs?.map((cost) => cost.Currency) ?? []);
  const selectedCurrency = $derived(
    currencies.includes(currency) ? currency : (currencies[0] ?? 'USD'),
  );
  const rows = $derived.by(() => {
    const groups = new Map<string, Statistics[]>();
    for (const agent of agents) {
      const key = group === 'model' ? agent.ModelID : agent.Agent;
      const values = groups.get(key) ?? [];
      values.push(agent.Statistics);
      groups.set(key, values);
    }
    const values = [...groups.entries()]
      .map(([id, statistics]) => ({
        id,
        label: group === 'model' ? usageModelLabel(id, models) : agentLabel(id),
        statistics: combineStatistics(statistics),
      }))
      .sort(
        (first, second) =>
          (usageValue(second.statistics, metric, selectedCurrency) ?? -Infinity) -
            (usageValue(first.statistics, metric, selectedCurrency) ?? -Infinity) ||
          first.id.localeCompare(second.id),
      );
    const leading = values.slice(0, 7).map((row) => ({
      label: row.label,
      value: usageValue(row.statistics, metric, selectedCurrency),
    }));
    if (values.length > 7)
      leading.push({
        label: `Other ${group === 'model' ? 'models' : 'workers'} (${values.length - 7})`,
        value: usageValue(
          combineStatistics(values.slice(7).map((row) => row.statistics)),
          metric,
          selectedCurrency,
        ),
      });
    return leading;
  });
  const pageEntries = $derived(agents.slice(page * 25, (page + 1) * 25));
</script>

<section class="min-w-0 space-y-4" aria-label="Worker usage breakdown">
  {#if agents.length}
    <div class="grid grid-cols-2 gap-3">
      <div class="min-w-0 space-y-2">
        <Label for={`${identifier}-metric`}>Compare</Label><SelectField
          id={`${identifier}-metric`}
          value={metric}
          options={usageMetrics}
          onValueChange={(value) => (metric = value as UsageMetric)}
        />
      </div>
      <div class="min-w-0 space-y-2">
        <Label for={`${identifier}-group`}>Group by</Label><SelectField
          id={`${identifier}-group`}
          value={group}
          options={[
            { value: 'model', label: 'Model' },
            { value: 'worker', label: 'Worker' },
          ]}
          onValueChange={(value) => (group = value)}
        />
      </div>
    </div>
    {#if metric === 'cost' && currencies.length > 1}<SelectField
        id={`${identifier}-currency`}
        ariaLabel="Worker cost currency"
        value={selectedCurrency}
        options={currencies.map((value) => ({ value, label: value }))}
        onValueChange={(value) => (currency = value)}
      />{/if}
    <MetricChart
      title={`${usageMetrics.find((option) => option.value === metric)?.label} by ${group}`}
      data={rows}
      series={[
        {
          key: 'value',
          label: usageMetrics.find((option) => option.value === metric)?.label ?? 'Usage',
          color: chartColors[0],
        },
      ]}
      {metric}
      currency={selectedCurrency}
      horizontal
      categoryLabels="below"
      legend={false}
      height={Math.max(120, rows.length * 34 + 32)}
      description={metric === 'tokens'
        ? 'All-time workspace usage. Uncached input + output; cache stays separate.'
        : metric === 'cost'
          ? `All-time recorded costs${totals.Costs?.some((cost) => cost.Currency === selectedCurrency && cost.Estimated) ? ' · includes estimates' : ''}. Missing prices stay unknown.`
          : 'All-time workspace usage.'}
    />
    <Disclosure title="Worker details"
      >{#each pageEntries as agent (`${agent.Agent}:${agent.ModelID}`)}<MemoryAgentUsage
          {agent}
          label={agentLabel(agent.Agent)}
        />{/each}
      {#if agents.length > 25}<div class="mt-3 flex justify-between gap-2">
          <Button variant="outline" class="h-11" disabled={page === 0} onclick={() => page--}
            >Previous</Button
          ><Button
            variant="outline"
            class="h-11"
            disabled={(page + 1) * 25 >= agents.length}
            onclick={() => page++}>Next</Button
          >
        </div>{/if}
    </Disclosure>
  {:else}<p class="text-sm text-muted-foreground">No worker calls yet.</p>{/if}
</section>
