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
    onRecord,
  }: {
    api: HarnessApi;
    record: ProcessRecord;
    activity: ToolActivity;
    onRecord?: (record: ProcessRecord) => void;
  } = $props();
  let feed = $state<ProcessFeed>();
  const processID = $derived(record.ID);
  const showConnectionStatus = $derived(
    record.Status === 'running' ||
      !(feed?.snapshot.chunks.length || activity.processNotice || activity.result),
  );
  $effect(() => {
    const client = api;
    processID;
    const current = untrack(() => new ProcessFeed(client, record, onRecord));
    feed = current;
    return () => current.close();
  });
</script>

{#if feed}
  <div
    class="flex flex-wrap items-center gap-x-2 text-[10px] text-muted-foreground"
    title={record.ID}
  >
    <span
      role="status"
      class:sr-only={feed.state === 'live' || feed.state === 'stopped' || !showConnectionStatus}
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
    {#if feed.state === 'unavailable' && showConnectionStatus}<Button
        variant="ghost"
        class="h-11 text-xs"
        onclick={() => feed?.reconnect()}><RefreshCw class="size-3" />Reconnect terminal</Button
      >{/if}
  </div>
  {#if !feed.canInput && !feed.snapshot.chunks.length && (activity.processNotice || activity.result?.Content?.length)}<ToolOutput
      {activity}
    />
  {:else}<TerminalOutput
      snapshot={feed.snapshot}
      interactive={feed.canInput}
      raw={feed.terminal}
      onInput={(data) => feed?.sendInput(data)}
      onResize={(columns, rows) => feed?.resize(columns, rows)}
      onInterrupt={() => feed?.interrupt()}
    />{/if}
  {#if feed.inputError}<p role="alert" class="text-xs text-muted-foreground">
      {feed.inputError}
    </p>{/if}
  {#if feed.error && showConnectionStatus}<p
      class="mt-1 whitespace-pre-wrap break-words text-xs text-muted-foreground"
    >
      {feed.error}
    </p>{/if}
{:else}<ToolOutput {activity} />{/if}
