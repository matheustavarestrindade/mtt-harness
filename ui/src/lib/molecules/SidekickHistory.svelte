<script lang="ts">
  import { Bot, ChevronRight, RefreshCw } from 'lucide-svelte';
  import { Button } from '../atoms/ui/button';
  import type { PluginActivityEntry } from '../atoms/plugin-activity';
  import SidekickSection from './SidekickSection.svelte';
  let {
    entries,
    more = false,
    loading = false,
    error = '',
    onMore,
    onRefresh,
  }: {
    entries: PluginActivityEntry[];
    more?: boolean;
    loading?: boolean;
    error?: string;
    onMore: () => void;
    onRefresh: () => void;
  } = $props();
</script>

{#if entries.length || more || error}
  <details class="assistant-section group min-w-0" aria-label="Earlier Sidekick context">
    <summary class="activity-toggle cursor-pointer list-none [&::-webkit-details-marker]:hidden"
      ><ChevronRight class="size-3 shrink-0 transition-transform group-open:rotate-90" /><Bot
        class="size-3.5 shrink-0"
      /><span>Earlier Sidekick context</span>{#if entries.length}<span
          class="ml-auto text-[10px] text-muted-foreground">{entries.length}</span
        >{/if}</summary
    >
    <div class="max-h-96 min-w-0 overflow-auto overscroll-contain">
      {#each entries as entry (entry.ID)}<SidekickSection {entry} />{/each}
      {#if error}<p role="status" class="px-2 py-1 text-xs text-muted-foreground">{error}</p>
        <Button
          variant="ghost"
          class="h-9 text-xs pointer-coarse:h-11"
          onclick={onRefresh}
          disabled={loading}><RefreshCw class="size-3" />Retry Sidekick history</Button
        >{/if}
      {#if more}<Button
          variant="ghost"
          class="h-9 text-xs pointer-coarse:h-11"
          onclick={onMore}
          disabled={loading}>{loading ? 'Loading…' : 'Older Sidekick entries'}</Button
        >{/if}
    </div>
  </details>
{/if}
