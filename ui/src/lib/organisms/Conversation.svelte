<script lang="ts">
  import { untrack } from 'svelte';
  import {
    Terminal,
    Sparkles,
    FolderOpen,
    ArrowDown,
    LoaderCircle,
    Cable,
    MessageSquarePlus,
    Play,
  } from 'lucide-svelte';
  import { Button } from '$lib/atoms/ui/button';
  import type { HarnessConsole } from '$lib/organisms/console.svelte';
  import { queuedMessageViews } from '../atoms/queued-messages';
  import { buildConversationEntries, type ToolActivity } from '$lib/atoms/transcript';
  import ToolActivityView from './ToolActivityView.svelte';
  import ProcessNoticeSection from '../molecules/ProcessNoticeSection.svelte';
  import SidekickSection from '../molecules/SidekickSection.svelte';
  import SidekickHistory from '../molecules/SidekickHistory.svelte';
  import { placePluginActivity } from '../atoms/plugin-activity';
  import AgentGroup from './AgentGroup.svelte';
  import MessageCard from '../molecules/MessageCard.svelte';
  import QueuedMessage from '../molecules/QueuedMessage.svelte';
  import PermissionCard from '../molecules/PermissionCard.svelte';
  import { HistoryScroll } from './history-scroll.svelte';
  let {
    console: workbench,
    busy = '',
    onConnection,
    onWorkspace,
    onSession,
    onResume,
    onCancelQueued,
    onDecision,
    onPrompt,
  }: {
    console: HarnessConsole;
    busy?: string;
    onConnection: () => void;
    onWorkspace: () => void;
    onSession: () => void;
    onResume: () => void;
    onCancelQueued: (identifier: string) => void;
    onDecision: (identifier: string, kind: 'allow' | 'deny') => void;
    onPrompt: (text: string) => void;
  } = $props();
  let feed: HTMLDivElement;
  const scroll = new HistoryScroll(untrack(() => workbench.history));
  $effect(() => () => scroll.dispose());
  const entries = $derived(
    buildConversationEntries(
      workbench.liveMessage && workbench.history.tailAttached
        ? [...workbench.messages, workbench.liveMessage]
        : workbench.messages,
      workbench.processes.records.values(),
    ),
  );
  const unplacedAgents = $derived(
    workbench.agents.unplaced(new Set(entries.map((entry) => entry.message.ID))),
  );
  const sidekick = $derived(placePluginActivity(entries, workbench.sidekick.entries));
  const queued = $derived(
    queuedMessageViews(
      workbench.receipts,
      workbench.status.messages ?? [],
      new Set(workbench.messages.map((message) => message.ID)),
      workbench.queuedPreviews,
    ),
  );
  $effect(() => {
    const sessionID = workbench.session?.ID ?? '';
    const changes =
      workbench.messages.length +
      workbench.sidekick.entries.length +
      workbench.permissions.length +
      queued.length +
      workbench.agents.children(workbench.session?.ID ?? '').length +
      (workbench.liveMessage?.Reasoning?.length ?? 0) +
      (workbench.liveMessage?.Content?.[0]?.Text.length ?? 0);
    workbench.history.version;
    workbench.history.loading;
    scroll.element = feed;
    scroll.track(sessionID, changes);
  });
  function updateScrollPosition() {
    scroll.onScroll();
  }
</script>

<div class="relative min-h-0 flex-1">
  <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions (Observe navigation input without replacing native keyboard or touch scrolling.) -->
  <div
    bind:this={feed}
    class="h-full min-w-0 overflow-y-auto overscroll-y-contain"
    onscroll={updateScrollPosition}
    onpointerdown={() => scroll.interact()}
    onwheel={(event) => {
      if (event.deltaY) scroll.interact(event.deltaY < 0 ? 'up' : 'down');
    }}
    ontouchstart={() => scroll.interact()}
    onkeydown={(event) => {
      if (['PageUp', 'ArrowUp', 'Home'].includes(event.key)) scroll.interact();
      else if (['PageDown', 'ArrowDown', 'End'].includes(event.key)) scroll.interact('down');
    }}
    role="region"
    aria-label="Conversation"
    data-history-count={workbench.history.messages.length}
    data-history-bytes={workbench.history.bytes}
    tabindex="0"
  >
    <div class="mx-auto flex min-h-full w-full max-w-4xl flex-col px-4 py-4 sm:px-6 sm:py-5">
      {#if (workbench.loading || workbench.history.loading) && !workbench.messages.length}<div
          class="grid flex-1 place-items-center py-16"
        >
          <span class="flex items-center gap-2 text-sm text-muted-foreground"
            ><LoaderCircle class="size-4 animate-spin" />Loading conversation…</span
          >
        </div>
      {:else if !workbench.messages.length && !queued.length && !workbench.liveMessage && !unplacedAgents.length && !workbench.sidekick.entries.length && !workbench.sidekick.error}
        <section class="flex flex-1 flex-col items-center justify-center py-8 text-center sm:py-12">
          <div
            class="quiet-grid mb-6 grid size-28 place-items-center rounded-3xl border border-border/50 bg-muted/30"
          >
            <div
              class="grid size-14 place-items-center rounded-2xl border border-border bg-secondary"
            >
              <Terminal class="size-7 text-primary" strokeWidth={1.5} />
            </div>
          </div>
          <p class="eyebrow mb-3 text-primary/70">Your next idea starts here</p>
          <h1 class="max-w-lg text-2xl font-medium tracking-tight text-foreground sm:text-3xl">
            {workbench.connection !== 'connected'
              ? 'Meet your coding workspace.'
              : !workbench.instance
                ? 'Give your work a home.'
                : !workbench.session
                  ? 'Start a fresh conversation.'
                  : 'What are we building today?'}
          </h1>
          <p class="mt-4 max-w-md text-sm leading-6 text-muted-foreground">
            {workbench.connection !== 'connected'
              ? 'Connect your harness to work with models, run tools, and follow each step from one place.'
              : !workbench.instance
                ? 'Create a workspace for a project directory on your harness server.'
                : !workbench.session
                  ? 'Choose a model and open a session. The harness takes care of the tools and the queue.'
                  : 'Ask a question, explore the project, or hand off a focused task. You can follow the tools as they run.'}
          </p>
          <div class="mt-7">
            {#if workbench.connection !== 'connected'}<Button
                class="h-11 px-5"
                onclick={onConnection}><Cable class="size-4" />Connect harness</Button
              >
            {:else if !workbench.instance}<Button class="h-11 px-5" onclick={onWorkspace}
                ><FolderOpen class="size-4" />Create workspace</Button
              >
            {:else if workbench.instance.Stopped}<Button
                class="h-11 px-5"
                onclick={onResume}
                disabled={busy === 'resume'}><Play class="size-4" />Resume workspace</Button
              >
            {:else if !workbench.session}<Button class="h-11 px-5" onclick={onSession}
                ><MessageSquarePlus class="size-4" />New session</Button
              >
            {:else}<div class="grid w-full max-w-xl gap-3 text-left sm:grid-cols-2">
                {#each [{ title: 'Explore the project', text: 'Read the project instructions and summarize the code structure.' }, { title: 'Find a starting point', text: 'Inspect the project and suggest one small, testable improvement. Explain it before making changes.' }] as suggestion}<button
                    onclick={() => onPrompt(suggestion.text)}
                    class="group rounded-xl border border-border bg-card/50 p-4 text-left transition-colors hover:border-primary/30 hover:bg-accent/40"
                    ><span class="mb-2 flex items-center gap-2 text-xs font-medium"
                      ><Sparkles class="size-3.5 text-primary/70" />{suggestion.title}</span
                    ><span class="text-xs leading-5 text-muted-foreground">{suggestion.text}</span
                    ></button
                  >{/each}
              </div>{/if}
          </div>
          <p class="mt-8 text-[10px] text-muted-foreground/70">
            API-first · Workspace-scoped · Built for focused work
          </p>
        </section>
      {:else}
        <div class="conversation-entries flex flex-col gap-1">
          {#if workbench.history.hasOlder}<Button
              variant="ghost"
              class="h-11 self-center text-xs"
              disabled={workbench.history.loading}
              onclick={() => void scroll.older()}>Load earlier messages</Button
            >{/if}
          {#if workbench.history.error}<p role="alert" class="text-xs text-muted-foreground">
              {workbench.history.error}
            </p>{/if}
          <SidekickHistory
            entries={sidekick.unplaced}
            more={!!workbench.sidekick.nextCursor}
            loading={workbench.sidekick.loading}
            error={workbench.sidekick.error}
            onMore={() => void workbench.sidekick.more()}
            onRefresh={() => void workbench.sidekick.refresh()}
          />
          {#each entries as entry (entry.message.ID)}
            <div data-history-id={entry.message.ID} data-history-seq={entry.message.Seq}>
              {#snippet tools(activity: ToolActivity)}
                {#if activity.call.Name === 'agent'}
                  {#if activity === entry.tools.find((tool) => tool.call.Name === 'agent')}
                    <AgentGroup
                      monitor={workbench.agents}
                      sessions={workbench.agents.children(
                        entry.message.SessionID,
                        entry.message.ID,
                      )}
                      models={workbench.models}
                      activities={entry.tools.filter((tool) => tool.call.Name === 'agent')}
                      running={workbench.status.running}
                    />
                  {/if}
                {:else}<ToolActivityView
                    {activity}
                    messageID={entry.message.ID}
                    processes={workbench.processes}
                    running={workbench.status.running}
                  />{/if}
              {/snippet}
              {#if entry.processNotice}<ProcessNoticeSection
                  notice={entry.processNotice}
                  record={entry.processRecord}
                />{:else}<MessageCard
                  message={entry.message}
                  tools={entry.tools}
                  renderTool={tools}
                  running={workbench.status.running}
                  streaming={entry.message.ID === workbench.liveMessage?.ID}
                />{/if}
              {#each sidekick.anchored.get(entry.message.ID) ?? [] as hint (hint.ID)}<SidekickSection
                  entry={hint}
                />{/each}
            </div>
          {/each}
          {#if workbench.history.hasNewer}<Button
              variant="ghost"
              class="h-11 self-center text-xs"
              disabled={workbench.history.loading}
              onclick={() => void scroll.newer()}>Load newer messages</Button
            >{/if}
          {#if workbench.liveTruncated && workbench.history.following}<p
              class="text-[10px] text-muted-foreground"
            >
              Earlier live output omitted. The saved response remains available in history.
            </p>{/if}
          {#if unplacedAgents.length}<AgentGroup
              monitor={workbench.agents}
              sessions={unplacedAgents}
              models={workbench.models}
            />{/if}
          {#each queued as entry (entry.id)}<QueuedMessage
              {entry}
              cancelling={workbench.cancellingQueued.has(entry.id)}
              onCancel={onCancelQueued}
              onLoad={(identifier) => void workbench.loadQueuedMessage(identifier)}
              loading={workbench.loadingQueued.has(entry.id)}
              error={workbench.queuedReadErrors.get(entry.id) ?? ''}
            />{/each}
        </div>
      {/if}
      {#if workbench.instance?.Stopped}<div
          class="mt-5 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-amber-200/15 bg-amber-200/5 p-4"
        >
          <p class="text-sm text-muted-foreground">
            This workspace is stopped. Its history is still available.
          </p>
          <Button variant="outline" class="h-11" disabled={busy === 'resume'} onclick={onResume}
            ><Play class="size-4" />Resume workspace</Button
          >
        </div>{/if}
      {#if workbench.session?.Completed}<p
          class="mt-5 rounded-xl border border-border bg-muted/25 p-4 text-sm text-muted-foreground"
        >
          This child agent has completed its task. Its conversation is read-only.
        </p>{/if}
      {#if workbench.status.error}<p
          role="alert"
          class="mt-5 break-words rounded-xl border border-destructive/25 bg-destructive/5 p-4 text-sm leading-6 text-destructive"
        >
          {workbench.status.error}
        </p>{/if}
      {#each workbench.permissions as request (request.ID)}<div class="mt-5">
          <PermissionCard {request} busy={busy === request.ID} {onDecision} />
        </div>{/each}
    </div>
  </div>
  {#if (!scroll.following || workbench.history.hasNewer) && workbench.messages.length}<Button
      size="icon"
      class="absolute right-5 bottom-4 size-11 rounded-full shadow-lg"
      aria-label="Scroll to latest message"
      onclick={() => void scroll.latest()}><ArrowDown class="size-4" /></Button
    >{/if}
</div>
