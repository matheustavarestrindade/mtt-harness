<script lang="ts">
  import { BarChart } from 'layerchart';
  import { scaleBand } from 'd3-scale';
  import * as Chart from '../../atoms/ui/chart';
  import type { TooltipPayload } from '../../atoms/ui/chart/chart-utils';
  import type { ChartDatum, ChartSeries, UsageMetric } from '../../atoms/usage';
  import { formatUsageDay, usageValueLabel } from '../../atoms/usage';
  let {
    data,
    series,
    metric = 'tokens',
    currency = '',
    horizontal = false,
    dates = false,
    height = 240,
    containerWidth = 0,
    categories = [],
  }: {
    data: ChartDatum[];
    series: ChartSeries[];
    metric?: UsageMetric;
    currency?: string;
    horizontal?: boolean;
    dates?: boolean;
    height?: number;
    containerWidth?: number;
    categories?: ChartSeries[];
  } = $props();
  const plotSeries = $derived(categories.length ? categories : series);
  const configuration = $derived(
    Object.fromEntries(
      plotSeries.map((item) => [item.key, { label: item.label, color: item.color }]),
    ),
  );
  const positive = $derived(
    Math.max(
      0,
      ...data.map((row) =>
        series.reduce(
          (sum, item) =>
            sum + Math.max(0, typeof row[item.key] === 'number' ? (row[item.key] as number) : 0),
          0,
        ),
      ),
    ),
  );
  const minimum = $derived(
    Math.min(
      0,
      ...data.map((row) =>
        series.reduce(
          (sum, item) =>
            sum + Math.min(0, typeof row[item.key] === 'number' ? (row[item.key] as number) : 0),
          0,
        ),
      ),
    ),
  );
  const maximum = $derived(positive || (!minimum ? 1 : 0));
  // The same chart renders in wide settings pages and narrow sidebars. Keep
  // category labels within a quarter of the actual container, not the viewport.
  const categoryPadding = $derived(containerWidth > 0 ? Math.min(130, containerWidth / 4) : 130);
  const labelStyle = {
    fontSize: '12px',
    fontFamily: 'var(--font-sans)',
    fontWeight: '400',
  } as CSSStyleDeclaration;
  const valueTicks = $derived(
    metric !== 'cost' && maximum - minimum < 5
      ? Array.from(
          { length: Math.ceil(maximum) - Math.floor(minimum) + 1 },
          (_, index) => Math.floor(minimum) + index,
        )
      : 4,
  );
  const chartData = $derived(
    data.map((row, index) => ({
      ...row,
      axisKey: dates ? row.label : `row-${index}`,
      ...(categories[index] ? { [categories[index].key]: row[series[0].key] } : {}),
    })),
  );
  const labelsByKey = $derived(new Map(chartData.map((row) => [row.axisKey, row.label])));
  const labels = $derived(chartData.map((row) => row.axisKey));
  const ticks = $derived(
    dates
      ? labels.filter((_, index) => index % Math.max(1, Math.ceil(labels.length / 5)) === 0)
      : undefined,
  );
  const formatAxis = (value: unknown) =>
    metric === 'cost'
      ? new Intl.NumberFormat(undefined, {
          notation: 'compact',
          maximumFractionDigits: Number(value) !== 0 && Math.abs(Number(value)) < 0.01 ? 6 : 2,
        }).format(Number(value))
      : usageValueLabel(Number(value), metric, currency, true);
  const formatCategory = (value: unknown) => {
    const label = labelsByKey.get(String(value)) ?? String(value);
    const characters = Array.from(label);
    return dates
      ? formatUsageDay(label, true)
      : characters.length > 18
        ? `${characters.slice(0, 17).join('')}…`
        : label;
  };
</script>

<Chart.Container
  config={configuration}
  class="min-w-0 w-full aspect-auto"
  style={`height:${height}px`}
>
  <BarChart
    data={chartData}
    series={plotSeries}
    orientation={horizontal ? 'horizontal' : 'vertical'}
    x={horizontal ? undefined : 'axisKey'}
    y={horizontal ? 'axisKey' : undefined}
    xScale={horizontal ? undefined : scaleBand().padding(0.2)}
    yScale={horizontal ? scaleBand().padding(0.35) : undefined}
    xDomain={horizontal ? [minimum, maximum] : undefined}
    yDomain={horizontal ? undefined : [minimum, maximum]}
    seriesLayout="stack"
    motion="none"
    padding={{
      left: categories.length
        ? minimum < 0 && metric === 'cost'
          ? 26
          : 8
        : horizontal
          ? categoryPadding
          : 42,
      right: categories.length ? (metric === 'cost' ? 26 : 16) : 12,
      top: 12,
      bottom: 28,
    }}
    props={{
      xAxis: {
        format: horizontal ? formatAxis : formatCategory,
        ticks: horizontal ? valueTicks : ticks,
      },
      yAxis: {
        format: horizontal ? formatCategory : formatAxis,
        ticks: categories.length ? [] : horizontal ? labels : valueTicks,
        tickLabelProps:
          horizontal && !categories.length
            ? { truncate: { maxWidth: Math.max(0, categoryPadding - 10), style: labelStyle } }
            : undefined,
      },
      tooltip: { context: { touchEvents: 'pan-y' } },
    }}
  >
    {#snippet tooltip({ context })}
      <Chart.Tooltip
        role="tooltip"
        hideLabel={categories.length > 0}
        label={dates
          ? formatUsageDay(String(context.tooltip.data?.label ?? data[0]?.label))
          : String(context.tooltip.data?.label ?? '')}
        class="max-w-[min(22rem,calc(100vw-2rem))] [overflow-wrap:anywhere]"
      >
        {#snippet formatter({
          value,
          name,
          item,
        }: {
          value: unknown;
          name: string;
          item: TooltipPayload;
        })}
          <span class="inline-block size-2.5 shrink-0 rounded-sm" style:background={item.color}
          ></span>
          <span class="min-w-0 flex-1 break-words text-muted-foreground">{name}</span>
          <span class="shrink-0 tabular-nums"
            >{usageValueLabel(typeof value === 'number' ? value : null, metric, currency)}</span
          >
        {/snippet}
      </Chart.Tooltip>
    {/snippet}
  </BarChart>
</Chart.Container>
