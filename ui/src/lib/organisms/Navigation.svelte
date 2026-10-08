<script lang="ts">
  import { tick } from 'svelte';
  import {
    FolderOpen,
    Plus,
    MessageSquare,
    Settings2,
    Circle,
    ChevronRight,
    ChevronsUpDown,
    Layers,
    X,
    LoaderCircle,
    RefreshCw,
    GitBranch,
    Trash2,
  } from 'lucide-svelte';
  import { Button } from '$lib/atoms/ui/button';
  import type { HarnessConsole } from '$lib/organisms/console.svelte';
  import type { Instance, Session } from '$lib/atoms/types';
  import { shortID, workspaceName } from '$lib/atoms/format';
  let {
    console: workbench,
    view,
    onWorkspaces,
    onClose,
    onSettings,
    onWorkspace,
    onSession,
    onSelectInstance,
    onSelectSession,
    onDeleteSession,
    onRefresh,
  }: {
    console: HarnessConsole;
    view: 'workspaces' | 'sessions';
    onWorkspaces: () => void;
    onClose?: () => void;
    onSettings: () => void;
    onWorkspace: () => void;
    onSession: () => void;
    onSelectInstance: (instance: Instance) => void;
    onSelectSession: (session: Session) => void;
    onDeleteSession: (session: Session) => void;
    onRefresh: () => void;
  } = $props();
  const showingSessions = $derived(view === 'sessions' && !!workbench.instance);
  const currentWorkspace = $derived(
    workbench.instance ? workspaceName(workbench.instance.Workspace) : 'Workspaces',
  );
  let navigation: HTMLElement | undefined;
  let workspaceSwitcher: HTMLButtonElement | null = $state(null);

  async function showWorkspaces() {
    const selectedID = workbench.instance?.ID;
    onWorkspaces();
    await tick();
    if (!navigation?.getClientRects().length || showingSessions) return;
    const selected = Array.from(
      navigation.querySelectorAll<HTMLButtonElement>('[data-workspace-id]'),
    ).find((button) => button.dataset.workspaceId === selectedID);
    selected?.focus();
  }

  async function showWorkspaceSessions(instance: Instance) {
    onSelectInstance(instance);
    await tick();
    if (
      showingSessions &&
      workbench.instance?.ID === instance.ID &&
      navigation?.getClientRects().length
    )
      workspaceSwitcher?.focus();
  }
</script>

<nav
  bind:this={navigation}
  aria-label="Workspaces and sessions"
  class="flex h-full min-h-0 min-w-0 flex-col bg-card"
>
  <header class="flex min-h-16 shrink-0 items-center gap-1 p-2">
    {#if showingSessions}
      <Button
        bind:ref={workspaceSwitcher}
        variant="ghost"
        class="h-12 min-w-0 flex-1 justify-start gap-2 rounded-lg px-2 text-left"
        aria-label="Switch workspace"
        title={`${workbench.instance?.Workspace} — switch workspace`}
        onclick={showWorkspaces}
      >
        <span
          class="grid size-8 shrink-0 place-items-center rounded-lg bg-primary text-primary-foreground"
          ><FolderOpen class="size-4" /></span
        >
        <span class="min-w-0 flex-1"
          ><span class="block truncate text-sm font-medium">{currentWorkspace}</span><span
            class="block truncate text-[11px] font-normal text-muted-foreground"
            >Switch workspace</span
          ></span
        >
        <ChevronsUpDown class="size-3.5 shrink-0 text-muted-foreground" />
      </Button>
    {:else}
      <div class="flex min-w-0 flex-1 items-center gap-2 px-2">
        <span
          class="grid size-8 shrink-0 place-items-center rounded-lg bg-primary text-primary-foreground"
          ><Layers class="size-4" /></span
        >
        <h2 class="truncate text-sm font-medium">Workspaces</h2>
      </div>
    {/if}
    {#if onClose}<Button
        variant="ghost"
        size="icon"
        class="icon-button shrink-0 text-muted-foreground"
        aria-label="Close navigation"
        onclick={onClose}><X class="size-4" /></Button
      >{/if}
  </header>

  <div class="shrink-0 px-3 pt-1 pb-3">
    {#if showingSessions}
      <Button
        variant="outline"
        class="h-9 w-full justify-start gap-2 rounded-md px-2.5 text-xs pointer-coarse:h-11"
        onclick={onSession}
        disabled={workbench.connection !== 'connected' || !!workbench.instance?.Stopped}
        ><Plus class="size-4" />New session</Button
      >
    {:else}
      <Button
        variant="outline"
        class="h-9 w-full justify-start gap-2 rounded-md px-2.5 text-xs pointer-coarse:h-11"
        onclick={onWorkspace}
        disabled={workbench.connection !== 'connected'}><Plus class="size-4" />New workspace</Button
      >
    {/if}
  </div>

  <div class="min-h-0 flex-1 overflow-y-auto overscroll-y-contain px-2 pb-3">
    {#if showingSessions}
      <h2 class="flex h-8 items-center px-2 text-xs font-medium text-muted-foreground">Sessions</h2>
      {#if !workbench.sessions.length}
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
      <div class="space-y-0.5">
        {#each workbench.sessions as session (session.ID)}
          <div
            class="group/session flex min-h-8 w-full min-w-0 items-center rounded-md pr-0.5 transition-colors pointer-coarse:min-h-11 {workbench
              .session?.ID === session.ID
              ? 'bg-accent text-accent-foreground'
              : 'text-muted-foreground hover:bg-muted hover:text-foreground'}"
          >
            <button
              class="flex h-8 min-w-0 flex-1 items-center gap-2 rounded-md px-2 text-left text-xs pointer-coarse:h-11"
              aria-current={workbench.session?.ID === session.ID ? 'page' : undefined}
              title={`${session.ID}\n${session.Model}`}
              onclick={() => onSelectSession(session)}
            >
              {#if session.Parent}<GitBranch class="size-3.5 shrink-0" />{:else}<MessageSquare
                  class="size-3.5 shrink-0"
                  strokeWidth={1.7}
                />{/if}
              <span class="min-w-0 flex-1 truncate"
                >{session.Parent ? 'Agent' : 'Session'} {shortID(session.ID)}</span
              >
            </button>
            <Button
              variant="ghost"
              size="icon"
              class="size-7 min-h-7 min-w-7 shrink-0 rounded-md p-0 text-muted-foreground hover:text-destructive pointer-coarse:size-11 pointer-coarse:min-h-11 pointer-coarse:min-w-11 pointer-fine:opacity-0 pointer-fine:group-hover/session:opacity-100 pointer-fine:group-focus-within/session:opacity-100"
              aria-label={`Delete session ${shortID(session.ID)}`}
              title="Delete session"
              onclick={() => onDeleteSession(session)}><Trash2 class="size-3.5" /></Button
            >
          </div>
        {/each}
      </div>
    {:else}
      <div class="mb-1 flex min-h-9 items-center justify-between pl-2">
        <p class="text-xs font-medium text-muted-foreground">Your workspaces</p>
        <Button
          variant="ghost"
          size="icon"
          class="icon-button text-muted-foreground"
          aria-label="Refresh workspaces"
          title="Refresh workspaces"
          onclick={onRefresh}
          disabled={workbench.connection !== 'connected'}><RefreshCw class="size-3.5" /></Button
        >
      </div>
      {#if !workbench.instances.length}<p class="px-2 py-3 text-xs leading-5 text-muted-foreground">
          {workbench.connection === 'connected'
            ? 'Create a workspace to begin.'
            : 'Connect to load your workspaces.'}
        </p>{/if}
      <div class="space-y-0.5">
        {#each workbench.instances as instance (instance.ID)}
          <button
            class="flex min-h-9 w-full min-w-0 items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors pointer-coarse:min-h-11 {workbench
              .instance?.ID === instance.ID
              ? 'bg-accent text-accent-foreground'
              : 'text-muted-foreground hover:bg-muted hover:text-foreground'}"
            aria-current={workbench.instance?.ID === instance.ID ? 'page' : undefined}
            data-workspace-id={instance.ID}
            title={instance.Workspace}
            onclick={() => showWorkspaceSessions(instance)}
          >
            <FolderOpen class="size-4 shrink-0" strokeWidth={1.7} /><span
              class="min-w-0 flex-1 truncate">{workspaceName(instance.Workspace)}</span
            >
            {#if instance.Stopped}<span
                class="size-1.5 shrink-0 rounded-full bg-muted-foreground"
                aria-label="Stopped"
              ></span>{/if}
            <ChevronRight class="size-3.5 shrink-0 opacity-50" />
          </button>
        {/each}
      </div>
    {/if}
  </div>
  <button
    class="flex min-h-16 items-center gap-3 border-t border-border px-5 text-left hover:bg-muted"
    onclick={onSettings}
  >
    <span class="relative grid size-8 place-items-center rounded-full bg-secondary"
      ><Settings2 class="size-4 text-muted-foreground" /><span
        class="absolute right-0 bottom-0 size-2 rounded-full ring-2 ring-card {workbench.connection ===
        'connected'
          ? 'bg-primary'
          : 'bg-muted-foreground'}"
      ></span></span
    >
    <span class="flex-1"
      ><span class="block text-xs font-medium">Settings</span><span
        class="mt-0.5 block text-[10px] text-muted-foreground"
        >{workbench.connection === 'connected'
          ? 'Harness connected'
          : 'API connection required'}</span
      ></span
    ><Circle class="size-3 text-muted-foreground/50" />
  </button>
</nav>
