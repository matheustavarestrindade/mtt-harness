<script lang="ts">
  import {
    Plus,
    MessageSquare,
    X,
    LoaderCircle,
    Trash2,
    ListTodo,
    BrainCircuit,
    Radio,
    ChevronRight,
  } from 'lucide-svelte';
  import { Button } from '$lib/atoms/ui/button';
  import * as Collapsible from '$lib/atoms/ui/collapsible';
  import ActivityIndicator from '../atoms/ActivityIndicator.svelte';
  import AgentStatusDot from '../atoms/AgentStatusDot.svelte';
  import { sessionChildren, sessionRoot } from '../atoms/session-tree';
  import WorkspaceSwitcher from '../molecules/WorkspaceSwitcher.svelte';
  import ConfigurationMenu from '../molecules/ConfigurationMenu.svelte';
  import type { SettingsScope } from '../atoms/settings';
  import type { HarnessConsole } from '$lib/organisms/console.svelte';
  import type { Instance, Session } from '$lib/atoms/types';
  import { shortID } from '$lib/atoms/format';
  let {
    console: workbench,
    onClose,
    onSettings,
    onSessionSettings,
    onWorkspace,
    onSession,
    onSelectInstance,
    onSelectSession,
    onSelectAgent,
    onDeleteSession,
    onUnlinkWorkspace,
    onRefresh,
    onTasks,
    onMemory,
    onActivity,
    hasActiveTasks = false,
  }: {
    console: HarnessConsole;
    onClose?: () => void;
    onSettings: (scope: SettingsScope, trigger: HTMLButtonElement | null) => void;
    onSessionSettings?: (trigger: HTMLButtonElement | null) => void;
    onWorkspace: () => void;
    onSession: () => void;
    onSelectInstance: (instance: Instance) => void;
    onSelectSession: (session: Session) => void;
    onSelectAgent: (session: Session) => void;
    onDeleteSession: (session: Session) => void;
    onUnlinkWorkspace: (instance: Instance) => void;
    onRefresh: () => void;
    onTasks?: () => void;
    onMemory?: () => void;
    onActivity?: () => void;
    hasActiveTasks?: boolean;
  } = $props();
  const processingSessionID = $derived(
    workbench.status.running && !workbench.permissions.length ? workbench.session?.ID : undefined,
  );
  let expanded = $state<Record<string, boolean>>({});
  const roots = $derived(workbench.sessions.filter((session) => !session.Parent));
  const unlinked = $derived(
    workbench.sessions.filter(
      (session) => session.Parent && !sessionRoot(session, workbench.sessions),
    ),
  );
</script>

{#snippet branch(session: Session, ancestors: string[])}
  {@const children = sessionChildren(workbench.sessions, session.ID).filter(
    (child) => !ancestors.includes(child.ID),
  )}
  {@const isAgent = !!session.Parent}
  {@const feed = workbench.agents.feeds.get(session.ID)}
  {@const active = feed?.active ?? false}
  {@const selected = isAgent
    ? workbench.agents.focusedID === session.ID
    : workbench.session?.ID === session.ID}
  <li data-session-node={session.ID} data-parent-id={session.Parent} class="min-w-0">
    <Collapsible.Root
      bind:open={() => expanded[session.ID] !== false, (value) => (expanded[session.ID] = value)}
    >
      <div
        class="group/session relative flex min-h-8 min-w-0 items-center rounded-md pr-0.5 pointer-coarse:min-h-11 {selected
          ? isAgent && !active
            ? 'bg-accent text-muted-foreground'
            : 'bg-accent text-accent-foreground'
          : isAgent && active
            ? 'text-foreground hover:bg-muted'
            : 'text-muted-foreground hover:bg-muted hover:text-foreground'}"
        data-agent-row={isAgent || undefined}
        data-active={active}
      >
        {#if isAgent}<span
            class="absolute -left-3 top-1/2 w-3 border-t border-border"
            aria-hidden="true"
          ></span>{/if}
        {#if children.length}
          <Collapsible.Trigger
            class="grid size-7 shrink-0 place-items-center rounded-md hover:bg-muted pointer-coarse:h-11 pointer-coarse:w-8"
            aria-label={`${expanded[session.ID] === false ? 'Expand' : 'Collapse'} agents for ${isAgent ? 'agent' : 'session'} ${shortID(session.ID)}`}
          >
            <ChevronRight
              class="size-3.5 transition-transform {expanded[session.ID] === false
                ? ''
                : 'rotate-90'}"
            />
          </Collapsible.Trigger>
        {/if}
        <Button
          variant="ghost"
          class="h-8 min-w-0 flex-1 justify-start gap-2 rounded-md border-0 px-2 text-left text-xs font-normal hover:bg-transparent pointer-coarse:h-11"
          aria-current={selected ? (isAgent ? 'location' : 'page') : undefined}
          aria-label={`${isAgent ? 'Agent' : 'Session'} ${shortID(session.ID)}`}
          title={`${workbench.agents.task(session.ID) || session.ID}\n${session.Model}${isAgent ? '\n' + (feed?.label ?? (session.Completed ? 'Inactive' : 'Status unavailable')) : ''}`}
          onclick={() => (isAgent ? onSelectAgent(session) : onSelectSession(session))}
        >
          {#if isAgent}<AgentStatusDot
              {active}
              label={feed?.label ?? (session.Completed ? 'Inactive' : 'Status unavailable')}
            />
          {:else if session.ID === processingSessionID}<span
              role="status"
              aria-label="Processing"
              data-slot="session-activity"
              class="inline-flex shrink-0"><ActivityIndicator class="size-3.5" /></span
            >
          {:else}<MessageSquare class="size-3.5 shrink-0" strokeWidth={1.7} />{/if}
          <span class="min-w-0 flex-1 truncate"
            >{isAgent ? 'Agent' : 'Session'} {shortID(session.ID)}</span
          >
        </Button>
        {#if !isAgent}<Button
            variant="ghost"
            size="icon"
            class="size-7 min-h-7 min-w-7 shrink-0 rounded-md p-0 text-muted-foreground hover:text-destructive pointer-coarse:size-11 pointer-coarse:min-h-11 pointer-coarse:min-w-11 pointer-fine:opacity-0 pointer-fine:group-hover/session:opacity-100 pointer-fine:group-focus-within/session:opacity-100"
            aria-label={`Delete session ${shortID(session.ID)}`}
            title="Delete session"
            onclick={() => onDeleteSession(session)}><Trash2 class="size-3.5" /></Button
          >{/if}
      </div>
      {#if children.length}<Collapsible.Content>
          {#if expanded[session.ID] !== false}
            <ul
              aria-label={`Agents of ${isAgent ? 'agent' : 'session'} ${shortID(session.ID)}`}
              class="ml-3 space-y-0.5 border-l border-border pl-3"
            >
              {#each children as child (child.ID)}{@render branch(child, [
                  ...ancestors,
                  session.ID,
                ])}{/each}
            </ul>
          {/if}
        </Collapsible.Content>{/if}
    </Collapsible.Root>
  </li>
{/snippet}

<nav aria-label="Workspaces and sessions" class="flex h-full min-h-0 min-w-0 flex-col bg-card">
  <header class="flex min-h-16 shrink-0 items-center gap-1 p-2">
    <WorkspaceSwitcher
      instances={workbench.instances}
      instance={workbench.instance}
      disabled={workbench.connection !== 'connected'}
      onSelect={onSelectInstance}
      onCreate={onWorkspace}
      {onRefresh}
      onUnlink={onUnlinkWorkspace}
    />
    {#if onClose}<Button
        variant="ghost"
        size="icon"
        class="icon-button shrink-0 text-muted-foreground"
        aria-label="Close navigation"
        onclick={onClose}><X class="size-4" /></Button
      >{/if}
  </header>

  <div class="shrink-0 px-3 pt-1 pb-3">
    <Button
      variant="outline"
      class="h-9 w-full justify-start gap-2 rounded-md px-2.5 text-xs pointer-coarse:h-11"
      onclick={onSession}
      disabled={workbench.connection !== 'connected' ||
        !workbench.instance ||
        !!workbench.instance.Stopped}><Plus class="size-4" />New session</Button
    >
  </div>

  <div class="min-h-0 flex-1 overflow-y-auto overscroll-y-contain px-2 pb-3">
    <h2 class="flex h-8 items-center px-2 text-xs font-medium text-muted-foreground">Sessions</h2>
    {#if !workbench.instance}
      <p class="px-2 py-3 text-xs leading-5 text-muted-foreground">
        Choose or create a workspace from the menu above.
      </p>
    {:else if !workbench.sessions.length}
      {#if workbench.loading}<p
          role="status"
          class="flex items-center gap-2 px-2 py-3 text-xs text-muted-foreground"
        >
          <LoaderCircle class="size-3.5 animate-spin" />Loading sessions…
        </p>
      {:else if workbench.error}<div class="space-y-2 px-2 py-3">
          <p role="alert" class="break-words text-xs text-muted-foreground">{workbench.error}</p>
          <Button
            variant="outline"
            class="h-11 text-xs"
            onclick={() => workbench.instance && onSelectInstance(workbench.instance)}
            >Retry loading sessions</Button
          >
        </div>
      {:else}<p class="px-2 py-3 text-xs leading-5 text-muted-foreground">
          No sessions in this workspace yet.
        </p>{/if}
    {/if}
    <ul aria-label="Sessions" class="space-y-0.5">
      {#each roots as session (session.ID)}{@render branch(session, [])}{/each}
    </ul>
    {#if unlinked.length}<p class="px-2 pt-3 text-xs text-muted-foreground">
        Agents with unavailable parents
      </p>
      <ul class="pl-3">
        {#each unlinked as session (session.ID)}{@render branch(session, [])}{/each}
      </ul>{/if}
  </div>
  {#if onTasks || onMemory || onActivity}
    <div
      class="shrink-0 border-t border-border p-2"
      style="padding-bottom: max(.5rem, env(safe-area-inset-bottom))"
      aria-label="Workspace tools"
    >
      <Button
        variant="ghost"
        class="h-11 w-full justify-start gap-3 px-3 text-sm"
        onclick={onTasks}
        disabled={!workbench.session}
      >
        <ListTodo class="size-4" />Tasks
        {#if hasActiveTasks}<span
            class="ml-auto size-1.5 rounded-full bg-primary"
            aria-hidden="true"
          ></span>{/if}
      </Button>
      <Button
        variant="ghost"
        class="h-11 w-full justify-start gap-3 px-3 text-sm"
        onclick={onMemory}
        disabled={!workbench.instance}
      >
        <BrainCircuit class="size-4" />Memory
      </Button>
      <Button
        variant="ghost"
        class="h-11 w-full justify-start gap-3 px-3 text-sm"
        onclick={onActivity}
        disabled={!workbench.session}
      >
        <Radio class="size-4" />Session activity
      </Button>
      <ConfigurationMenu
        variant="row"
        workspaceAvailable={!!workbench.instance}
        {onSettings}
        {onSessionSettings}
      />
    </div>
  {:else}<ConfigurationMenu
      variant="footer"
      workspaceAvailable={!!workbench.instance}
      connected={workbench.connection === 'connected'}
      {onSettings}
      {onSessionSettings}
    />{/if}
</nav>
