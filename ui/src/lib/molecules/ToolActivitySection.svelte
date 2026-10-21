<script lang="ts">
  import { ChevronRight, ArrowRightToLine, ArrowLeftFromLine } from 'lucide-svelte';
  import type { Snippet } from 'svelte';
  import type { ToolActivity } from '../atoms/transcript';
  import { toolActivityLabel } from '../atoms/file-actions';
  import ToolInput from './ToolInput.svelte';
  import ToolOutput from './ToolOutput.svelte';
  import * as Collapsible from '../atoms/ui/collapsible';
  let {
    activity,
    running = false,
    unmatched = false,
    renderOutput,
    status,
  }: {
    activity: ToolActivity;
    running?: boolean;
    unmatched?: boolean;
    renderOutput?: Snippet;
    status?: string;
  } = $props();
  let open = $state(false);
  const label = $derived(toolActivityLabel(activity.call));
</script>

<Collapsible.Root
  bind:open
  class="assistant-section tool-activity flex min-w-0 flex-col"
  role="group"
  aria-label={`Tool ${activity.call.Name}`}
  data-call-id={activity.call.ID}
>
  <Collapsible.Trigger class="activity-toggle w-full shrink-0">
    <ChevronRight
      class="size-3.5 shrink-0 text-tool-call-accent transition-transform {open ? 'rotate-90' : ''}"
    /><span
      class="min-w-0 flex-1 truncate font-mono font-medium"
      title={`${activity.call.Name}: ${label}`}>{label}</span
    >
    <span class="shrink-0 text-[10px] text-muted-foreground"
      >{status ?? (activity.result ? 'Result' : running ? 'Waiting' : 'No result')}</span
    >
  </Collapsible.Trigger>
  <Collapsible.Content
    class="tool-details mt-1 min-w-0 space-y-2 border border-border bg-card/70 p-2"
  >
    {#if open}
      {#if unmatched}<p class="mt-2 text-xs text-muted-foreground">
          The original tool call is not in this history.
        </p>
      {:else}
        <section
          aria-label="Tool input"
          class="grid min-w-0 grid-cols-[.875rem_minmax(0,1fr)] items-start gap-2"
        >
          <ArrowRightToLine aria-hidden="true" class="mt-1.5 size-3.5 text-muted-foreground" />
          <div class="min-w-0"><ToolInput call={activity.call} /></div>
        </section>
      {/if}
      <section
        aria-label="Tool output"
        class="grid min-w-0 grid-cols-[.875rem_minmax(0,1fr)] items-start gap-2 border-t border-border pt-2"
      >
        <ArrowLeftFromLine aria-hidden="true" class="mt-1.5 size-3.5 text-muted-foreground" />
        <div class="min-w-0">
          {#if renderOutput}{@render renderOutput()}
          {:else if activity.result?.Content?.length}<ToolOutput {activity} />
          {:else}<p class="mt-2 text-xs leading-5 text-muted-foreground">
              {activity.result
                ? 'No output.'
                : running
                  ? 'Waiting for the tool result…'
                  : 'No result was recorded for this call.'}
            </p>{/if}
        </div>
      </section>
      <p class="mt-2 break-all font-mono text-[10px] text-muted-foreground">
        {activity.call.ID}
      </p>
    {/if}
  </Collapsible.Content>
</Collapsible.Root>
