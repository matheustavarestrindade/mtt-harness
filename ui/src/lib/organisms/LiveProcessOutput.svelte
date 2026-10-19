<script lang="ts">
  import { untrack } from 'svelte';
  import { RefreshCw } from 'lucide-svelte';
  import { Button } from '../atoms/ui/button';
  import type { ProcessRecord } from '../atoms/processes';
  import type { ToolActivity } from '../atoms/transcript';
  import { HarnessApi } from '../molecules/api/client';
  import TerminalOutput from '../molecules/content/TerminalOutput.svelte';
  import ToolOutput from '../molecules/ToolOutput.svelte';
  import { ProcessFeed } from './process-feed.svelte';
  let {
    api,
    record,
    activity,
  }: { api: HarnessApi; record: ProcessRecord; activity: ToolActivity } = $props();
  let feed = $state<ProcessFeed>();
  const processID = $derived(record.ID);
  $effect(() => {
    const client = api;
    processID;
    if (untrack(() => record.Status === 'stopped' && activity.processNotice?.kind === 'exit')) {
      feed = undefined;
      return;
    }
    const current = untrack(() => new ProcessFeed(client, record));
    feed = current;
    return () => current.close();
  });
</script>

{#if feed}
  <div class="flex flex-wrap items-center gap-x-2 text-[10px] text-muted-foreground">
    <span class="break-all font-mono">Process {record.ID}</span>
    <span role="status"
      >{feed.state === 'stopped'
        ? feed.exit
          ? `Exited ${feed.exit.Code}`
          : 'Stopped'
        : feed.state === 'live'
          ? record.Status === 'stopped'
            ? 'Saved output'
            : 'Live'
          : feed.state === 'retrying'
            ? 'Reconnecting…'
            : feed.state === 'unavailable'
              ? 'Live output unavailable'
              : 'Connecting…'}</span
    >
    {#if feed.state === 'unavailable'}<Button
        variant="ghost"
        class="h-11 text-xs"
        onclick={() => feed?.reconnect()}><RefreshCw class="size-3" />Reconnect terminal</Button
      >{/if}
  </div>
  {#if (feed.state === 'unavailable' || !feed.snapshot.chunks.length) && (activity.processNotice || activity.result?.Content?.length)}<ToolOutput
      {activity}
    />
  {:else}<TerminalOutput snapshot={feed.snapshot} />{/if}
  {#if feed.error}<p class="mt-1 whitespace-pre-wrap break-words text-xs text-muted-foreground">
      {feed.error}
    </p>{/if}
{:else if activity.processNotice}<ToolOutput {activity} />{/if}
