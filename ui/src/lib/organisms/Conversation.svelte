<script lang="ts">
  import { tick } from 'svelte';
  import {
    Terminal,
    Sparkles,
    FolderOpen,
    ArrowDown,
    LoaderCircle,
    X,
    Cable,
    MessageSquarePlus,
    Play,
  } from 'lucide-svelte';
  import { Button } from '$lib/atoms/ui/button';
  import type { HarnessConsole } from '$lib/organisms/console.svelte';
  import { messageText, shortID } from '$lib/atoms/format';
  import MessageCard from '../molecules/MessageCard.svelte';
  import PermissionCard from '../molecules/PermissionCard.svelte';
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
  let following = $state(true);
  let previousSession = '';
  $effect(() => {
    const sessionID = workbench.session?.ID ?? '';
    const changes =
      workbench.messages.length + workbench.permissions.length + workbench.receipts.length;
    if (sessionID !== previousSession) {
      previousSession = sessionID;
      following = true;
    }
    if (following && changes >= 0)
      void tick().then(() => {
        if (feed) feed.scrollTop = feed.scrollHeight;
      });
  });
  function scroll() {
    if (feed) following = feed.scrollHeight - feed.scrollTop - feed.clientHeight < 100;
  }
</script>

<div class="relative min-h-0 flex-1">
  <!-- svelte-ignore a11y_no_noninteractive_tabindex (Keyboard users need focus here for Page Up/Down scrolling.) -->
  <div
    bind:this={feed}
    class="h-full min-w-0 overflow-y-auto overscroll-y-contain"
    onscroll={scroll}
    role="region"
    aria-label="Conversation"
    tabindex="0"
  >
    <div class="mx-auto flex min-h-full w-full max-w-4xl flex-col px-4 py-5 sm:px-8 sm:py-8">
      {#if workbench.loading && !workbench.messages.length}<div
          class="grid flex-1 place-items-center py-16"
        >
          <span class="flex items-center gap-2 text-sm text-muted-foreground"
            ><LoaderCircle class="size-4 animate-spin" />Loading conversation…</span
          >
        </div>
      {:else if !workbench.messages.length && !workbench.receipts.length}
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
        <div class="space-y-5">
          {#each workbench.messages as message (message.ID)}<MessageCard {message} />{/each}
          {#each workbench.receipts as receipt (receipt.ID)}<div
              class="rounded-xl border border-dashed border-border bg-muted/20 p-4"
            >
              <div class="mb-2 flex items-center gap-2 text-xs text-muted-foreground">
                <LoaderCircle class="size-3.5 animate-spin" />Message accepted
              </div>
              <p class="message-copy text-foreground/75">{messageText(receipt)}</p>
            </div>{/each}
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
      {#if workbench.status.running}<div
          role="status"
          class="mt-6 flex items-center gap-3 py-2 text-xs text-muted-foreground"
        >
          <span class="flex gap-1"
            ><span class="size-1 rounded-full bg-primary"></span><span
              class="size-1 rounded-full bg-primary/60"
            ></span><span class="size-1 rounded-full bg-primary/30"></span></span
          >Harness is working{workbench.permissions.length ? ' · waiting for permission' : '…'}
        </div>{/if}
      {#if workbench.status.error}<p
          role="alert"
          class="mt-5 break-words rounded-xl border border-destructive/25 bg-destructive/5 p-4 text-sm leading-6 text-destructive"
        >
          {workbench.status.error}
        </p>{/if}
      {#each workbench.permissions as request (request.ID)}<div class="mt-5">
          <PermissionCard {request} busy={busy === request.ID} {onDecision} />
        </div>{/each}
      {#if workbench.status.queued > 0}
        <section aria-label="Queued messages" class="mt-6 rounded-xl border border-border p-3">
          <h3 class="eyebrow px-1 py-2">{workbench.status.queued} queued</h3>
          {#each workbench.status.messages ?? [] as identifier (identifier)}<div
              class="flex min-h-12 items-center gap-3 border-t border-border px-1"
            >
              <span class="min-w-0 flex-1 truncate text-xs text-muted-foreground"
                >{workbench.receipts.find((entry) => entry.ID === identifier)?.Content?.[0]?.Text ||
                  `Message ${shortID(identifier)}`}</span
              ><Button
                variant="ghost"
                size="icon"
                class="icon-button"
                aria-label={`Cancel queued message ${shortID(identifier)}`}
                disabled={busy === identifier}
                onclick={() => onCancelQueued(identifier)}><X class="size-4" /></Button
              >
            </div>{/each}
        </section>
      {/if}
    </div>
  </div>
  {#if !following && workbench.messages.length}<Button
      size="icon"
      class="absolute right-5 bottom-4 size-11 rounded-full shadow-lg"
      aria-label="Scroll to latest message"
      onclick={() => {
        following = true;
        feed.scrollTo({ top: feed.scrollHeight, behavior: 'smooth' });
      }}><ArrowDown class="size-4" /></Button
    >{/if}
</div>
