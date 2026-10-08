<script lang="ts">
  import { ListTodo, X } from 'lucide-svelte';
  import { Button } from '../atoms/ui/button';
  import { shortID } from '../atoms/format';
  import type { Session, TaskState } from '../atoms/types';
  import TaskProgress from '../molecules/TaskProgress.svelte';

  let {
    session,
    state,
    error = '',
    onClose,
  }: {
    session: Session | null;
    state: TaskState | null;
    error?: string;
    onClose: () => void;
  } = $props();
  const active = $derived(Boolean(state?.Doing) || Boolean(state?.Todo.length));
</script>

<section
  id="session-tasks-panel"
  aria-label="Session tasks"
  class="flex h-full min-h-0 min-w-0 flex-col bg-background"
>
  <header class="flex min-h-14 shrink-0 items-center gap-2 border-b border-border px-4">
    <ListTodo class="size-4 text-muted-foreground" />
    <h2 class="min-w-0 flex-1 text-sm font-medium">Tasks</h2>
    <Button
      variant="ghost"
      size="icon"
      class="icon-button text-muted-foreground"
      aria-label="Close tasks panel"
      onclick={onClose}><X class="size-4" /></Button
    >
  </header>
  <div class="min-h-0 flex-1 overflow-y-auto overscroll-contain">
    {#if session}
      <p class="border-b border-border px-4 py-3 text-xs text-muted-foreground" title={session.ID}>
        Session {shortID(session.ID)}
      </p>
      {#key session.ID}<TaskProgress {state} {error} />{/key}
      {#if !active && !error}
        <p role="status" class="px-4 py-5 text-sm text-muted-foreground">
          {state ? 'No active tasks.' : 'Loading task progress…'}
        </p>
      {/if}
    {:else}<p class="px-4 py-5 text-sm text-muted-foreground">
        Select a session to see its tasks.
      </p>{/if}
  </div>
</section>
