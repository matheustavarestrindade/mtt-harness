<script lang="ts">
  import { ArrowDownToLine, ArrowUpFromLine, Coins, Database } from 'lucide-svelte';
  import type { Statistics } from '$lib/atoms/types';
  import type { RequestUsage } from '$lib/atoms/request-usage';
  import { formatTokenCount, costLabel } from '$lib/atoms/format';
  let {
    statistics,
    requestUsage,
  }: { statistics: Statistics | null; requestUsage: RequestUsage | null } = $props();
  const tokens = (value: number | null | undefined) =>
    value == null ? '—' : formatTokenCount(value);
  const percentage = (value: number | null | undefined) =>
    value == null ? '—' : `${value.toFixed(1)}%`;
  const contextTitle = $derived(
    requestUsage?.contextTokens != null
      ? `Latest provider-reported request in this session: ${requestUsage.inputTokens?.toLocaleString()} input + ${requestUsage.outputTokens?.toLocaleString()} output tokens. Reasoning is included in output. Measured at ${requestUsage.reportedAt}${requestUsage.modelID ? ` using ${requestUsage.modelID}` : ''}. Later messages and tool results are not measured yet.${requestUsage.contextLimit === null ? ' Model limit unavailable.' : ' Limit is from the model catalog.'}`
      : 'Context usage unavailable: the latest response has not reported token usage. This is not cumulative session usage.',
  );
  const cacheTitle = $derived(
    requestUsage?.inputTokens != null
      ? `Latest request: ${requestUsage.cacheReadTokens?.toLocaleString()} cache-read tokens / ${requestUsage.inputTokens.toLocaleString()} total input tokens. Cache-write tokens: ${requestUsage.cacheWriteTokens?.toLocaleString()}. Cache writes are not cache hits.`
      : 'Cache usage unavailable for the latest response.',
  );
</script>

<div
  class="flex min-w-0 flex-wrap items-center justify-center gap-x-3 gap-y-1 px-2 py-2 text-[10px] text-muted-foreground sm:justify-start sm:gap-x-5"
  aria-label="Session usage"
>
  <span class="flex items-center gap-1.5" title={contextTitle} aria-label="Last reported context"
    ><ArrowDownToLine class="size-3" />Last context
    <span class="font-mono text-foreground/80"
      >{tokens(requestUsage?.contextTokens)}{#if requestUsage?.contextLimit != null}
        {' / '}{tokens(requestUsage.contextLimit)}
        <span class="text-muted-foreground">({percentage(requestUsage.contextPercentage)})</span
        >{/if}</span
    ></span
  >
  <span
    class="flex items-center gap-1.5"
    aria-label="Last response output"
    title="Provider-reported output for the latest response in this session, including reasoning tokens. Not a conversation total."
    ><ArrowUpFromLine class="size-3" />Out
    <span class="font-mono text-foreground/80">{tokens(requestUsage?.outputTokens)}</span></span
  >
  <span class="flex items-center gap-1.5" aria-label="Last request cache hit" title={cacheTitle}
    ><Database class="size-3" />Cache
    <span class="font-mono text-foreground/80">{percentage(requestUsage?.cacheHitPercentage)}</span
    ></span
  >
  <span
    class="flex items-center gap-1.5"
    aria-label="Conversation cost total"
    title="Cumulative known cost for this conversation and its child agents. Unknown prices are excluded. Catalog-estimated costs are marked ≈."
    ><Coins class="size-3" />Total<span class="font-mono text-foreground/80"
      >{statistics?.Cost?.Estimated || statistics?.Costs?.some((cost) => cost.Estimated)
        ? '≈ '
        : ''}{costLabel(statistics)}</span
    ></span
  >
</div>
