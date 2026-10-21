<script lang="ts">
  import { untrack } from 'svelte';
  import { ArrowDown, ArrowLeft, ChevronDown, GitBranch, LoaderCircle } from 'lucide-svelte';
  import { Button } from '../atoms/ui/button';
  import * as Collapsible from '../atoms/ui/collapsible';
  import { Toggle } from '../atoms/ui/toggle';
  import AgentStatusDot from '../atoms/AgentStatusDot.svelte';
  import { agentResult } from '../atoms/agent-activity';
  import { shortID } from '../atoms/format';
  import { topLevelAgent } from '../atoms/session-tree';
  import { buildConversationEntries, type ToolActivity } from '../atoms/transcript';
  import type { Model, Session } from '../atoms/types';
  import MessageCard from '../molecules/MessageCard.svelte';
  import RichText from '../molecules/content/RichText.svelte';
  import MessageContent from '../molecules/content/MessageContent.svelte';
  import ToolActivityView from './ToolActivityView.svelte';
  import ProcessNoticeSection from '../molecules/ProcessNoticeSection.svelte';
  import SidekickSection from '../molecules/SidekickSection.svelte';
  import SidekickHistory from '../molecules/SidekickHistory.svelte';
  import { placePluginActivity } from '../atoms/plugin-activity';
  import PermissionCard from '../molecules/PermissionCard.svelte';
  import type { AgentMonitor } from './agent-monitor.svelte';
  import { HistoryScroll } from './history-scroll.svelte';

  let { session, monitor, models }: { session: Session; monitor: AgentMonitor; models: Model[] } =
    $props();
  let open = $state(untrack(() => monitor.feeds.get(session.ID)?.active ?? false));
  let disclosureChanged = false;
  let selectedID = $state(untrack(() => session.ID));
  let showTask = $state(false);
  let scroll = $state<HistoryScroll>();
  let output = $state<HTMLDivElement>();
  let previousAgent = '';
  let previousFocusRevision = -1;
  let permissionBusy = $state('');
  let actionError = $state('');
  const feed = $derived(monitor.feeds.get(selectedID));
  const selected = $derived(feed?.session ?? session);
  const task = $derived(monitor.task(selectedID));
  const model = $derived(models.find((model) => model.ID === selected.Model));
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
  const sidekick = $derived(placePluginActivity(entries, feed?.sidekick.entries ?? []));
  $effect(() => {
    if (!disclosureChanged && feed?.active) open = true;
  });

  $effect(() => {
    if (monitor.focusRevision === previousFocusRevision) return;
    const focused = monitor.focusedID;
    if (!focused || topLevelAgent(focused, monitor.rootID, monitor.sessions)?.ID !== session.ID)
      return;
    previousFocusRevision = monitor.focusRevision;
    selectedID = focused;
    open = true;
  });
  $effect(() => {
    if (open && feed) return untrack(() => feed.enablePreview());
  });
  $effect(() => {
    if (!feed) return;
    const current = new HistoryScroll(feed.history);
    scroll = current;
    return () => current.dispose();
  });
  $effect(() => {
    const changes =
      (feed?.messages.length ?? 0) +
      (feed?.sidekick.entries.length ?? 0) +
      (feed?.liveMessage?.Reasoning?.length ?? 0) +
      (feed?.liveMessage?.Content?.[0]?.Text.length ?? 0) +
      (feed?.liveMessage?.ToolCalls?.length ?? 0);
    if (previousAgent !== selectedID) {
      previousAgent = selectedID;
      showTask = false;
    }
    feed?.history.version;
    feed?.history.loading;
    if (scroll) {
      scroll.element = output;
      if (open) scroll.track(selectedID, changes);
    }
  });

  async function decide(requestID: string, kind: 'allow' | 'deny') {
    if (!feed) return;
    permissionBusy = requestID;
    actionError = '';
    try {
      await feed.decide(requestID, kind);
    } catch (failure) {
      actionError = failure instanceof Error ? failure.message : 'Cannot resolve permission.';
    } finally {
      permissionBusy = '';
    }
  }
</script>

<Collapsible.Root
  bind:open={
    () => open,
    (next) => {
      disclosureChanged = true;
      open = next;
      if (
        !next &&
        monitor.focusedID &&
        topLevelAgent(monitor.focusedID, monitor.rootID, monitor.sessions)?.ID === session.ID
      )
        monitor.clearFocus();
    }
  }
  id={`agent-panel-${session.ID}`}
  data-agent-id={session.ID}
  aria-label={`Agent ${shortID(session.ID)}`}
  role="region"
  class={`agent-panel flex min-w-0 flex-col overflow-hidden border border-border bg-card/30 ${open ? 'h-80' : ''}`}
>
  <div
    class="flex min-h-10 shrink-0 items-center gap-2 border-b border-border px-2 pointer-coarse:min-h-11"
  >
    {#if selectedID !== session.ID}
      <Button
        variant="ghost"
        size="icon"
        class="size-8 shrink-0 pointer-coarse:size-11"
        aria-label="Back to parent agent"
        onclick={() => monitor.focus(selected.Parent)}><ArrowLeft class="size-3.5" /></Button
      >
    {:else}<GitBranch class="size-3.5 shrink-0 text-muted-foreground" />{/if}
    <Collapsible.Trigger
      data-agent-heading
      class="flex min-h-10 min-w-0 flex-1 items-center gap-2 text-left text-xs pointer-coarse:min-h-11"
      aria-label={`${open ? 'Collapse' : 'Expand'} agent ${shortID(session.ID)}`}
    >
      <span class="min-w-0 flex-1 truncate" title={task || selected.ID}
        >Agent {shortID(selected.ID)}</span
      >
      <AgentStatusDot active={feed?.active ?? false} label={feed?.label ?? 'Inactive'} />
      <span class="shrink-0 text-[10px] text-muted-foreground">{feed?.label ?? 'Inactive'}</span>
      <ChevronDown
        class="size-3.5 shrink-0 text-muted-foreground transition-transform {open
          ? ''
          : '-rotate-90'}"
      />
    </Collapsible.Trigger>
  </div>
  <Collapsible.Content class="flex min-h-0 flex-1 flex-col">
    {#if open}
      <div
        class="flex min-h-9 shrink-0 items-center gap-2 border-b border-border px-2 text-[10px] text-muted-foreground"
      >
        <span class="min-w-0 flex-1 truncate" title={feed?.taskState?.Doing?.Description || task}
          >{feed?.taskState?.Doing?.Title || task.split('\n')[0] || 'Delegated task'}</span
        >
        <span class="max-w-[35%] truncate" title={selected.Model}
          >{model?.Name || selected.Model.split('/').slice(1).join('/') || selected.Model}</span
        >
        <Toggle
          bind:pressed={showTask}
          class="h-9 min-h-9 shrink-0 px-1 text-[10px] pointer-coarse:h-11"
          aria-label="Show agent task">Task</Toggle
        >
      </div>
      <div class="relative min-h-0 flex-1">
        <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions (Observe navigation input without replacing native keyboard or touch scrolling.) -->
        <div
          bind:this={output}
          role="region"
          aria-label={`Agent ${shortID(selectedID)} output`}
          tabindex="0"
          class="h-full min-w-0 overflow-y-auto overscroll-contain p-2 text-xs"
          onscroll={() => {
            if (!showTask) scroll?.onScroll();
          }}
          onpointerdown={() => scroll?.interact()}
          onwheel={(event) => {
            if (event.deltaY) scroll?.interact(event.deltaY < 0 ? 'up' : 'down');
          }}
          ontouchstart={() => scroll?.interact()}
          onkeydown={(event) => {
            if (['PageUp', 'ArrowUp', 'Home'].includes(event.key)) scroll?.interact();
            else if (['PageDown', 'ArrowDown', 'End'].includes(event.key)) scroll?.interact('down');
          }}
        >
          {#if showTask}
            <p class="whitespace-pre-wrap break-words text-xs leading-5">
              {task || 'Task details are not available.'}
            </p>
          {:else}
            {#if (feed?.loading || feed?.history.loading) && !entries.length}<p
                class="flex items-center gap-2 py-2 text-muted-foreground"
              >
                <LoaderCircle class="size-3.5 animate-spin" />Loading agent output…
              </p>{/if}
            {#if feed?.history.hasOlder}<Button
                variant="ghost"
                class="h-11 text-xs"
                disabled={feed.history.loading}
                onclick={() => void scroll?.older()}>Load earlier agent messages</Button
              >{/if}
            {#if feed?.history.error}<p role="alert" class="text-xs text-muted-foreground">
                {feed.history.error}
              </p>{/if}
            {#if feed}<SidekickHistory
                entries={sidekick.unplaced}
                more={!!feed.sidekick.nextCursor}
                loading={feed.sidekick.loading}
                error={feed.sidekick.error}
                onMore={() => void feed?.sidekick.more()}
                onRefresh={() => void feed?.sidekick.refresh()}
              />{/if}
            {#each entries as entry (entry.message.ID)}
              <div data-history-id={entry.message.ID}>
                {#snippet tools(activity: ToolActivity)}
                  {@const result = agentResult(activity.call)}
                  {#if result !== null}
                    <section class="assistant-section" aria-label="Agent result">
                      <RichText text={result} embedded />
                    </section>
                  {:else if activity.call.Name === 'agent'}
                    {#if activity === entry.tools.find((tool) => tool.call.Name === 'agent')}
                      {@const children = monitor.children(selectedID, entry.message.ID)}
                      {#each children as child (child.ID)}
                        {@const childFeed = monitor.feeds.get(child.ID)}
                        <Button
                          variant="ghost"
                          class="h-auto min-h-10 w-full min-w-0 justify-start gap-2 rounded-none border-0 border-l border-border pl-3 text-left text-xs pointer-coarse:min-h-11"
                          onclick={() => monitor.focus(child.ID)}
                          aria-label={`View agent ${shortID(child.ID)}`}
                        >
                          <GitBranch class="size-3.5 shrink-0" /><span
                            class="min-w-0 flex-1 truncate">Agent {shortID(child.ID)}</span
                          >
                          <AgentStatusDot
                            active={childFeed?.active ?? false}
                            label={childFeed?.label ?? 'Inactive'}
                          />
                        </Button>
                      {:else}
                        {#each entry.tools.filter((tool) => tool.call.Name === 'agent') as delegation (delegation.call.ID)}
                          {#if delegation.result}<MessageContent
                              content={delegation.result.Content}
                              mode="plain"
                              embedded
                            />
                          {:else}<p class="py-1 text-xs text-muted-foreground">
                              {feed?.active
                                ? 'Delegating a task…'
                                : 'No child agent is linked to this call.'}
                            </p>{/if}
                        {/each}
                      {/each}
                    {/if}
                  {:else if feed}<ToolActivityView
                      {activity}
                      messageID={entry.message.ID}
                      processes={feed.processes}
                      running={feed.active}
                    />{/if}
                {/snippet}
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
                {#each sidekick.anchored.get(entry.message.ID) ?? [] as hint (hint.ID)}<SidekickSection
                    entry={hint}
                  />{/each}
              </div>
            {/each}
            {#if feed?.history.hasNewer}<Button
                variant="ghost"
                class="h-11 text-xs"
                disabled={feed.history.loading}
                onclick={() => void scroll?.newer()}>Load newer agent messages</Button
              >{/if}
            {#if !feed?.loading && !entries.length}<p class="py-2 text-xs text-muted-foreground">
                No agent output yet.
              </p>{/if}
            {#if feed?.liveTruncated}<p class="text-[10px] text-muted-foreground">
                The live preview shows the latest 100,000 characters. Saved output is loaded after
                the response.
              </p>{/if}
            {#if feed?.error}<div class="space-y-2 py-2">
                <p role="alert" class="break-words text-xs text-destructive">{feed.error}</p>
                <Button
                  variant="outline"
                  class="h-9 text-xs pointer-coarse:h-11"
                  onclick={() => feed?.refresh()}>Retry agent output</Button
                >
              </div>{/if}
            {#if actionError}<p role="alert" class="break-words text-xs text-destructive">
                {actionError}
              </p>{/if}
            {#each feed?.permissions ?? [] as request (request.ID)}<PermissionCard
                {request}
                busy={permissionBusy === request.ID}
                onDecision={decide}
              />{/each}
          {/if}
        </div>
        {#if ((scroll && !scroll.following) || feed?.history.hasNewer) && !showTask}<Button
            variant="secondary"
            size="icon"
            class="absolute right-2 bottom-2 size-9 pointer-coarse:size-11"
            aria-label="Scroll to latest agent output"
            onclick={() => void scroll?.latest()}><ArrowDown class="size-4" /></Button
          >{/if}
      </div>
    {/if}
  </Collapsible.Content>
</Collapsible.Root>
