<script lang="ts">
  import { untrack } from 'svelte';
  import { ArrowDown, LoaderCircle } from 'lucide-svelte';
  import { Button } from '../atoms/ui/button';
  import ActivityIndicator from '../atoms/ActivityIndicator.svelte';
  import { buildConversationEntries, type ToolActivity } from '../atoms/transcript';
  import type { HarnessApi } from '../molecules/api/client';
  import MessageCard from '../molecules/MessageCard.svelte';
  import ProcessNoticeSection from '../molecules/ProcessNoticeSection.svelte';
  import ToolActivityView from './ToolActivityView.svelte';
  import { AgentFeed } from './agent-feed.svelte';
  import { HistoryScroll } from './history-scroll.svelte';

  let { api, sessionID }: { api: HarnessApi; sessionID: string } = $props();
  // Activity records refresh while the same worker is running. Stabilize the
  // primitive session/API identity before owning its feed and scroll lifetime.
  const scopeAPI = $derived(api);
  const scopeSessionID = $derived(sessionID);
  let feed = $state<AgentFeed>(),
    scroll = $state<HistoryScroll>(),
    output = $state<HTMLDivElement>(),
    error = $state('');
  const entries = $derived(
    buildConversationEntries(
      feed
        ? feed.liveMessage && feed.history.tailAttached
          ? [...feed.messages, feed.liveMessage]
          : feed.messages
        : [],
      feed?.processes.records.values() ?? [],
    ),
  );
  $effect(() => {
    const currentAPI = scopeAPI,
      identifier = scopeSessionID,
      controller = new AbortController();
    let current: AgentFeed | undefined,
      historyScroll: HistoryScroll | undefined,
      release: (() => void) | undefined;
    feed = undefined;
    scroll = undefined;
    error = '';
    void currentAPI
      .session(identifier, controller.signal)
      .then((session) => {
        if (controller.signal.aborted) return;
        current = new AgentFeed(
          currentAPI,
          session,
          () => {},
          () => {},
        );
        historyScroll = new HistoryScroll(current.history);
        feed = current;
        scroll = historyScroll;
        release = untrack(() => current!.enablePreview());
      })
      .catch((failure) => {
        if (!controller.signal.aborted)
          error = failure instanceof Error ? failure.message : 'Cannot load reviewer activity.';
      });
    return () => {
      controller.abort();
      release?.();
      historyScroll?.dispose();
      current?.close();
    };
  });
  $effect(() => {
    const changes =
      (feed?.messages.length ?? 0) +
      (feed?.liveMessage?.Reasoning?.length ?? 0) +
      (feed?.liveMessage?.Content?.[0]?.Text.length ?? 0) +
      (feed?.liveMessage?.ToolCalls?.length ?? 0);
    feed?.history.version;
    feed?.history.loading;
    if (scroll) {
      scroll.element = output;
      scroll.track(sessionID, changes);
    }
  });
</script>

<section
  aria-label="Watcher live activity"
  class="flex h-80 min-w-0 flex-col overflow-hidden border border-border bg-card/30"
  data-watcher-worker={sessionID}
>
  <div
    class="flex min-h-9 shrink-0 items-center gap-2 border-b border-border px-2 text-[10px] text-muted-foreground"
  >
    {#if feed?.active}<ActivityIndicator />{/if}<span class="min-w-0 flex-1 truncate"
      >{feed?.taskState?.Doing?.Title || 'Reviewer activity'}</span
    ><span role="status">{feed?.label ?? 'Connecting'}</span>
  </div>
  <div class="relative min-h-0 flex-1">
    <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions (Observe native scrolling without replacing keyboard navigation.) -->
    <div
      bind:this={output}
      role="region"
      aria-label="Watcher output"
      tabindex="0"
      class="h-full min-w-0 overflow-y-auto overscroll-contain p-2 text-xs"
      onscroll={() => scroll?.onScroll()}
      onpointerdown={() => scroll?.interact()}
      onwheel={(event) => {
        if (event.deltaY) scroll?.interact(event.deltaY < 0 ? 'up' : 'down');
      }}
      ontouchstart={() => scroll?.interact()}
      onkeydown={(event) => {
        if (['PageUp', 'ArrowUp', 'Home'].includes(event.key)) scroll?.interact('up');
        else if (['PageDown', 'ArrowDown', 'End'].includes(event.key)) scroll?.interact('down');
      }}
    >
      {#if (!feed || feed.loading) && !entries.length && !error}<p
          class="flex items-center gap-2 py-2 text-muted-foreground"
        >
          <LoaderCircle class="size-3.5 animate-spin" />Loading reviewer activity…
        </p>{/if}
      {#if feed?.history.hasOlder}<Button
          variant="ghost"
          class="h-11 text-xs"
          disabled={feed.history.loading}
          onclick={() => void scroll?.older()}>Load earlier reviewer messages</Button
        >{/if}
      {#each entries as entry (entry.message.ID)}<div data-history-id={entry.message.ID}>
          {#snippet tools(activity: ToolActivity)}{#if feed}<ToolActivityView
                {activity}
                messageID={entry.message.ID}
                processes={feed.processes}
                running={feed.active}
              />{/if}{/snippet}
          {#if entry.processNotice}<ProcessNoticeSection
              notice={entry.processNotice}
              record={entry.processRecord}
            />{:else}<MessageCard
              message={entry.message}
              tools={entry.tools}
              renderTool={tools}
              running={feed?.active ?? false}
              streaming={entry.message.ID === feed?.liveMessage?.ID}
            />{/if}
        </div>{/each}
      {#if feed?.history.hasNewer}<Button
          variant="ghost"
          class="h-11 text-xs"
          disabled={feed.history.loading}
          onclick={() => void scroll?.newer()}>Load newer reviewer messages</Button
        >{/if}
      {#if feed && !feed.loading && !entries.length}<p class="py-2 text-muted-foreground">
          No reviewer output yet.
        </p>{/if}
      {#if feed?.liveTruncated}<p class="text-[10px] text-muted-foreground">
          The live preview contains the latest 100,000 characters. Saved output loads after the
          response.
        </p>{/if}
      {#if error || feed?.error || feed?.history.error}<p
          role="alert"
          class="break-words text-destructive"
        >
          {error || feed?.error || feed?.history.error}
        </p>{/if}
    </div>
    {#if scroll && (!scroll.following || feed?.history.hasNewer)}<Button
        variant="secondary"
        size="icon"
        class="absolute right-2 bottom-2 size-9 pointer-coarse:size-11"
        aria-label="Latest reviewer output"
        onclick={() => void scroll?.latest()}><ArrowDown class="size-4" /></Button
      >{/if}
  </div>
</section>
