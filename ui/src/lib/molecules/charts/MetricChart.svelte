<script lang="ts">
  import type { ChartDatum, ChartSeries, UsageMetric } from '../../atoms/usage';
  import { chartCategorySeries, formatUsageDay, usageValueLabel } from '../../atoms/usage';
  import Disclosure from '../Disclosure.svelte';
  let {
    title,
    data,
    series,
    description = '',
    metric = 'tokens',
    currency = '',
    horizontal = false,
    dates = false,
    height = 240,
    legend = true,
    categoryLabels = 'axis',
  }: {
    title: string;
    data: ChartDatum[];
    series: ChartSeries[];
    description?: string;
    metric?: UsageMetric;
    currency?: string;
    horizontal?: boolean;
    dates?: boolean;
    height?: number;
    legend?: boolean;
    categoryLabels?: 'axis' | 'below';
  } = $props();
  type ChartComponent = (typeof import('./BarChartView.svelte'))['default'];
  let Renderer = $state<ChartComponent | null>(null),
    error = $state('');
  let containerWidth = $state(0);
  const heading = $props.id();
  const categories = $derived(
    horizontal && categoryLabels === 'below' && series.length === 1
      ? chartCategorySeries(data)
      : [],
  );
  const hasValues = $derived(
    data.some((row) => series.some((item) => typeof row[item.key] === 'number')),
  );
  $effect(() => {
    if (!data.length || !series.length || !hasValues || Renderer) return;
    let disposed = false;
    void import('./BarChartView.svelte')
      .then((module) => {
        if (!disposed) Renderer = module.default;
      })
      .catch(() => {
        if (!disposed) error = 'Chart unavailable. Values are available in View data.';
      });
    return () => {
      disposed = true;
    };
  });
</script>

<section class="min-w-0 space-y-3" aria-label={title} data-metric-chart>
  <div class="space-y-1">
    <h3 id={heading} class="text-sm font-medium">{title}</h3>
    {#if description}<p class="text-xs leading-5 text-muted-foreground">{description}</p>{/if}
  </div>
  {#if !data.length || !series.length || !hasValues}<p class="py-6 text-sm text-muted-foreground">
      {metric === 'cost' ? 'No cost reported for this selection.' : 'No usage for this selection.'}
    </p>
  {:else}
    <div
      bind:clientWidth={containerWidth}
      role="img"
      aria-labelledby={heading}
      aria-label={`${title}. Exact values are available in View data.`}
      class="min-w-0"
      style={`min-height:${height}px`}
    >
      {#if Renderer}<Renderer
          {data}
          {series}
          {metric}
          {currency}
          {horizontal}
          {dates}
          {height}
          {containerWidth}
          {categories}
        />{:else if error}<p role="status" class="text-xs text-muted-foreground">
          {error}
        </p>{:else}<p role="status" class="py-8 text-xs text-muted-foreground">
          Loading chart…
        </p>{/if}
    </div>
    {#if categories.length}<ul
        class="flex min-w-0 flex-wrap gap-x-4 gap-y-2 text-xs text-muted-foreground"
        aria-label={`${title} categories`}
      >
        {#each categories as category (category.key)}<li
            class="flex min-w-0 max-w-full items-start gap-1.5"
          >
            <span class="mt-0.5 size-2.5 shrink-0 rounded-sm" style:background={category.color}
            ></span>
            <span class="min-w-0 break-words [overflow-wrap:anywhere]">{category.label}</span>
          </li>{/each}
      </ul>{/if}
    {#if legend && !categories.length && series.length > 1}<ul
        class="flex min-w-0 flex-wrap gap-x-4 gap-y-2 text-xs text-muted-foreground"
        aria-label={`${title} legend`}
      >
        {#each series as item (item.key)}<li class="flex min-w-0 max-w-full items-center gap-1.5">
            <span class="size-2.5 shrink-0 rounded-sm" style:background={item.color}></span><span
              class="min-w-0 break-words [overflow-wrap:anywhere]">{item.label}</span
            >
          </li>{/each}
      </ul>{/if}
    <Disclosure title="View data" class="text-xs text-muted-foreground">
      <div class="max-h-72 overflow-auto">
        <table class="w-full border-collapse text-left text-xs tabular-nums">
          <caption class="sr-only">{title} values</caption><thead
            ><tr
              ><th class="sticky top-0 bg-background p-2">{dates ? 'Day (UTC)' : 'Category'}</th
              >{#each series as item (item.key)}<th
                  class="sticky top-0 bg-background p-2 text-right">{item.label}</th
                >{/each}</tr
            ></thead
          ><tbody
            >{#each data as row, index (row.id ?? index)}<tr class="border-t border-border"
                ><th class="p-2 font-normal">{dates ? formatUsageDay(row.label) : row.label}</th
                >{#each series as item (item.key)}<td class="p-2 text-right whitespace-nowrap"
                    >{usageValueLabel(
                      typeof row[item.key] === 'number' ? (row[item.key] as number) : null,
                      metric,
                      currency,
                    )}</td
                  >{/each}</tr
              >{/each}</tbody
          >
        </table>
      </div>
    </Disclosure>
  {/if}
</section>
