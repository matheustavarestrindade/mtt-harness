<script lang="ts">
  import { ArrowDownToLine, ArrowUpFromLine, Coins, Database } from 'lucide-svelte';
  import type { Statistics } from '$lib/atoms/types';
  import { formatTokenCount, costLabel } from '$lib/atoms/format';
  let { statistics }: { statistics: Statistics | null } = $props();
</script>

<div
  class="flex min-w-0 flex-wrap items-center justify-center gap-x-4 gap-y-1 px-2 py-2 text-[10px] text-muted-foreground sm:justify-start sm:gap-x-5"
  aria-label="Session usage"
>
  <span
    class="flex items-center gap-1.5"
    title="Input includes uncached, cache-read, and cache-write tokens"
    ><ArrowDownToLine class="size-3" />Input
    <span class="font-mono text-foreground/80"
      >{statistics
        ? formatTokenCount(statistics.Input + statistics.CacheRead + statistics.CacheWrite)
        : '—'}</span
    ></span
  >
  <span class="flex items-center gap-1.5"
    ><ArrowUpFromLine class="size-3" />Output
    <span class="font-mono text-foreground/80"
      >{statistics ? formatTokenCount(statistics.Output) : '—'}</span
    ></span
  >
  <span class="hidden items-center gap-1.5 sm:flex"
    ><Database class="size-3" />Cache
    <span class="font-mono text-foreground/80"
      >{statistics ? `${statistics.CacheHitPercentage.toFixed(1)}%` : '—'}</span
    ></span
  >
  <span class="flex items-center gap-1.5"
    ><Coins class="size-3" /><span class="font-mono text-foreground/80"
      >{costLabel(statistics)}</span
    ></span
  >
</div>
