<script lang="ts">
  import {
    FolderOpen,
    Plus,
    MessageSquare,
    Settings2,
    Circle,
    ChevronRight,
    Terminal,
    RefreshCw,
    GitBranch,
  } from 'lucide-svelte';
  import { Button } from '$lib/atoms/ui/button';
  import { Separator } from '$lib/atoms/ui/separator';
  import Brand from '../atoms/Brand.svelte';
  import type { HarnessConsole } from '$lib/organisms/console.svelte';
  import type { Instance, Session } from '$lib/atoms/types';
  import { shortID, workspaceName } from '$lib/atoms/format';
  let {
    console: workbench,
    onSettings,
    onWorkspace,
    onSession,
    onSelectInstance,
    onSelectSession,
    onRefresh,
  }: {
    console: HarnessConsole;
    onSettings: () => void;
    onWorkspace: () => void;
    onSession: () => void;
    onSelectInstance: (instance: Instance) => void;
    onSelectSession: (session: Session) => void;
    onRefresh: () => void;
  } = $props();
</script>

<nav aria-label="Workspaces and sessions" class="flex h-full min-h-0 flex-col bg-card">
  <div class="px-5 pt-6 pb-5"><Brand /></div>
  <div class="px-4 pb-5">
    <Button
      class="h-11 w-full justify-start gap-2 border-primary/25 bg-primary/10 text-primary hover:bg-primary/15"
      variant="outline"
      onclick={onWorkspace}
      disabled={workbench.connection !== 'connected'}
      ><Plus class="size-4" />New workspace<span class="ml-auto text-xs text-primary/60">↗</span
      ></Button
    >
  </div>
  <div class="flex min-h-0 flex-1 flex-col overflow-y-auto px-3">
    <div class="mb-1 flex items-center justify-between pl-2">
      <h2 class="eyebrow">Workspaces</h2>
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
    {#if !workbench.instances.length}<p class="px-2 py-4 text-xs leading-5 text-muted-foreground">
        {workbench.connection === 'connected'
          ? 'Create a workspace to begin.'
          : 'Connect to load your workspaces.'}
      </p>{/if}
    <div class="space-y-1">
      {#each workbench.instances as instance (instance.ID)}
        <button
          class="group flex min-h-12 w-full min-w-0 items-center gap-2.5 rounded-lg px-3 py-2 text-left text-sm transition-colors {workbench
            .instance?.ID === instance.ID
            ? 'bg-accent text-accent-foreground'
            : 'text-muted-foreground hover:bg-muted hover:text-foreground'}"
          aria-current={workbench.instance?.ID === instance.ID ? 'page' : undefined}
          title={instance.Workspace}
          onclick={() => onSelectInstance(instance)}
        >
          <FolderOpen
            class="size-4 shrink-0 {workbench.instance?.ID === instance.ID ? 'text-primary' : ''}"
            strokeWidth={1.7}
          />
          <span class="min-w-0 flex-1 truncate">{workspaceName(instance.Workspace)}</span>
          {#if instance.Stopped}<span
              class="size-1.5 rounded-full bg-amber-400"
              aria-label="Stopped"
            ></span>{:else if workbench.instance?.ID === instance.ID}<ChevronRight
              class="size-3.5 shrink-0 opacity-50"
            />{/if}
        </button>
      {/each}
    </div>
    <Separator class="my-5 opacity-70" />
    <div class="mb-1 flex items-center justify-between pl-2">
      <h2 class="eyebrow">Sessions</h2>
      <Button
        variant="ghost"
        size="icon"
        class="icon-button text-muted-foreground"
        aria-label="New session"
        title="New session"
        onclick={onSession}
        disabled={!workbench.instance || workbench.instance.Stopped}><Plus class="size-4" /></Button
      >
    </div>
    {#if !workbench.sessions.length}<p class="px-2 py-4 text-xs leading-5 text-muted-foreground">
        Your conversations will appear here.
      </p>{/if}
    <div class="space-y-1 pb-5">
      {#each workbench.sessions as session (session.ID)}
        <button
          class="flex min-h-14 w-full min-w-0 items-center gap-2.5 rounded-lg px-3 py-2 text-left transition-colors {workbench
            .session?.ID === session.ID
            ? 'bg-accent text-accent-foreground ring-1 ring-inset ring-border'
            : 'text-muted-foreground hover:bg-muted'}"
          aria-current={workbench.session?.ID === session.ID ? 'page' : undefined}
          onclick={() => onSelectSession(session)}
        >
          {#if session.Parent}<GitBranch class="size-4 shrink-0" />{:else}<MessageSquare
              class="size-4 shrink-0"
              strokeWidth={1.6}
            />{/if}
          <span class="min-w-0 flex-1"
            ><span class="block truncate text-xs font-medium"
              >{session.Parent ? 'Agent' : 'Session'} {shortID(session.ID)}</span
            ><span class="mt-1 block truncate text-[10px] text-muted-foreground"
              >{session.Model}</span
            ></span
          >
          {#if workbench.session?.ID === session.ID}<span
              class="size-1.5 shrink-0 rounded-full bg-primary"
            ></span>{/if}
        </button>
      {/each}
    </div>
  </div>
  <div class="mx-4 mb-4 rounded-xl border border-border bg-background/35 p-3.5">
    <div class="mb-1.5 flex items-center gap-2 text-xs font-medium">
      <Terminal class="size-3.5 text-primary" />Your tools. Your workspace.
    </div>
    <p class="text-[11px] leading-5 text-muted-foreground">
      The harness runs the work.<br />This console keeps you in the loop.
    </p>
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
