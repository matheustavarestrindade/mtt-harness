<script lang="ts">
  import {
    Plus,
    MessageSquare,
    Settings2,
    Circle,
    X,
    LoaderCircle,
    GitBranch,
    Trash2,
    ListTodo,
    BrainCircuit,
    Radio,
  } from 'lucide-svelte';
  import { Button } from '$lib/atoms/ui/button';
  import WorkspaceSwitcher from '../molecules/WorkspaceSwitcher.svelte';
  import type { HarnessConsole } from '$lib/organisms/console.svelte';
  import type { Instance, Session } from '$lib/atoms/types';
  import { shortID } from '$lib/atoms/format';
  let {
    console: workbench,
    onClose,
    onSettings,
    onWorkspace,
    onSession,
    onSelectInstance,
    onSelectSession,
    onDeleteSession,
    onRefresh,
    onTasks,
    onMemory,
    onActivity,
    hasActiveTasks = false,
  }: {
    console: HarnessConsole;
    onClose?: () => void;
    onSettings: () => void;
    onWorkspace: () => void;
    onSession: () => void;
    onSelectInstance: (instance: Instance) => void;
    onSelectSession: (session: Session) => void;
    onDeleteSession: (session: Session) => void;
    onRefresh: () => void;
    onTasks?: () => void;
    onMemory?: () => void;
    onActivity?: () => void;
    hasActiveTasks?: boolean;
  } = $props();
</script>

<nav aria-label="Workspaces and sessions" class="flex h-full min-h-0 min-w-0 flex-col bg-card">
  <header class="flex min-h-16 shrink-0 items-center gap-1 p-2">
    <WorkspaceSwitcher
      instances={workbench.instances}
      instance={workbench.instance}
      disabled={workbench.connection !== 'connected'}
      onSelect={onSelectInstance}
      onCreate={onWorkspace}
      {onRefresh}
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
      <Button
        variant="ghost"
        class="h-11 w-full justify-start gap-3 px-3 text-sm"
        onclick={onSettings}
      >
        <Settings2 class="size-4" />Settings
      </Button>
    </div>
  {:else}<button
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
    </button>{/if}
</nav>
