<script lang="ts">
  import { ChevronDown, Terminal } from 'lucide-svelte';
  import type { ToolActivity } from '../atoms/transcript';
  import { fileLanguage } from '../atoms/content';
  import MessageContent from './content/MessageContent.svelte';
  import CodeBlock from './content/CodeBlock.svelte';
  let {
    activity,
    running = false,
    unmatched = false,
  }: { activity: ToolActivity; running?: boolean; unmatched?: boolean } = $props();
  let open = $state(false);
  const language = $derived.by(() => {
    const input = activity.call.Input;
    if (
      activity.call.Name !== 'read' ||
      !input ||
      typeof input !== 'object' ||
      !('path' in input) ||
      typeof input.path !== 'string'
    )
      return 'text';
    return fileLanguage(input.path);
  });
</script>

<section
  class="tool-activity my-3 min-w-0 overflow-hidden rounded-xl border border-border bg-card/40"
  role="group"
  aria-label={`Tool ${activity.call.Name}`}
  data-call-id={activity.call.ID}
>
  <button
    type="button"
    class="flex min-h-12 w-full min-w-0 items-center gap-2 px-3 py-2 text-left text-xs"
    aria-expanded={open}
    onclick={() => (open = !open)}
  >
    <Terminal class="size-3.5 shrink-0 text-muted-foreground" /><span
      class="min-w-0 flex-1 truncate font-mono font-medium">{activity.call.Name}</span
    >
    <span class="shrink-0 text-[10px] text-muted-foreground"
      >{activity.result ? 'Result' : running ? 'Waiting' : 'No result'}</span
    ><ChevronDown
      class="size-3.5 shrink-0 text-muted-foreground transition-transform {open
        ? 'rotate-180'
        : ''}"
    />
  </button>
  {#if open}
    <div class="min-w-0 space-y-4 border-t border-border/60 px-3 py-4 sm:px-4">
      {#if unmatched}<p class="mt-3 text-xs text-muted-foreground">
          The original tool call is not in this history.
        </p>
      {:else}
        <section aria-label="Tool input" class="min-w-0">
          <h3 class="eyebrow">Input</h3>
          <CodeBlock
            text={JSON.stringify(activity.call.Input, null, 2) ?? 'null'}
            language="json"
            embedded
          />
        </section>
      {/if}
      <section aria-label="Tool output" class="min-w-0 border-t border-border/60 pt-4">
        <h3 class="eyebrow">Output</h3>
        {#if activity.result?.Content?.length}<MessageContent
            content={activity.result.Content}
            mode={language === 'markdown' ? 'markdown' : 'code'}
            {language}
            embedded
          />
        {:else}<p class="mt-3 text-xs leading-6 text-muted-foreground">
            {activity.result
              ? 'No output.'
              : running
                ? 'Waiting for the tool result…'
                : 'No result was recorded for this call.'}
          </p>{/if}
      </section>
      <p class="mt-3 break-all font-mono text-[10px] text-muted-foreground">
        Call {activity.call.ID}
      </p>
    </div>
  {/if}
</section>
