<script lang="ts">
  import { Check, ChevronDown, Circle, CircleDot, ListTodo, X } from 'lucide-svelte';
  import type { TaskState } from '$lib/atoms/types';

  let { state: taskState, error = '' }: { state: TaskState | null; error?: string } = $props();
  let expanded = $state(true);
  const items = $derived(taskState?.Todo ?? []);
  const done = $derived(items.filter((item) => item.Status === 'done').length);
  const active = $derived(items.length > 0 || Boolean(taskState?.Doing));
  const statusLabels = {
    pending: 'Pending',
    in_progress: 'In progress',
    done: 'Done',
    cancelled: 'Cancelled',
  };
  $effect(() => {
    if (!active) expanded = true;
  });
</script>

{#if active || error}
  <section aria-label="Task progress" class="mb-2 min-w-0 border border-border bg-card/40 text-xs">
    {#if active}
      <button
        type="button"
        class="flex min-h-10 w-full items-center gap-2 px-3 py-2 text-left hover:bg-accent/30 focus-visible:outline-2 focus-visible:outline-ring"
        aria-label="Toggle task progress"
        aria-expanded={expanded}
        onclick={() => (expanded = !expanded)}
      >
        <ListTodo class="size-3.5 shrink-0 text-muted-foreground" />
        <span class="min-w-0 flex-1 break-words font-medium"
          >{taskState?.Doing?.Title ?? 'Tasks'}</span
        >
        {#if items.length}<span role="status" class="shrink-0 text-[10px] text-muted-foreground"
            >{done}/{items.length} done</span
          >{/if}
        <ChevronDown
          class={`size-3.5 shrink-0 text-muted-foreground transition-transform ${expanded ? '' : '-rotate-90'}`}
        />
      </button>
      {#if expanded}
        <div class="max-h-44 overflow-y-auto border-t border-border px-3 py-2">
          {#if taskState?.Doing}
            <p class="mb-2 whitespace-pre-wrap break-words leading-5 text-muted-foreground">
              {taskState.Doing.Description}
            </p>
          {/if}
          {#if items.length}
            <ul class="space-y-1" aria-label="TODO items">
              {#each items as item (item.ID)}
                <li class="flex min-h-7 items-start gap-2 py-1" data-status={item.Status}>
                  {#if item.Status === 'done'}<Check class="mt-0.5 size-3.5 shrink-0" />
                  {:else if item.Status === 'cancelled'}<X
                      class="mt-0.5 size-3.5 shrink-0 text-muted-foreground"
                    />
                  {:else if item.Status === 'in_progress'}<CircleDot
                      class="mt-0.5 size-3.5 shrink-0"
                    />
                  {:else}<Circle class="mt-0.5 size-3.5 shrink-0 text-muted-foreground" />{/if}
                  <span
                    class={`min-w-0 flex-1 break-words ${item.Status === 'done' || item.Status === 'cancelled' ? 'text-muted-foreground line-through' : ''}`}
                    >{item.Title}</span
                  >
                  <span class="shrink-0 text-[10px] text-muted-foreground"
                    >{statusLabels[item.Status]}</span
                  >
                </li>
              {/each}
            </ul>
          {/if}
        </div>
      {/if}
    {/if}
    {#if error}<p role="status" class="break-words px-3 py-2 text-muted-foreground">
        Task progress unavailable: {error}
      </p>{/if}
  </section>
{/if}
