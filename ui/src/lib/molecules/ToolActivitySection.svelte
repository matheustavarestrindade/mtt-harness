<script lang="ts">
  import { ChevronRight } from 'lucide-svelte';
  import type { ToolActivity } from '../atoms/transcript';
  import { fileLanguage } from '../atoms/content';
  import { plainFileReadPath, toolActivityLabel } from '../atoms/file-actions';
  import MessageContent from './content/MessageContent.svelte';
  import CodeBlock from './content/CodeBlock.svelte';
  let {
    activity,
    running = false,
    unmatched = false,
  }: { activity: ToolActivity; running?: boolean; unmatched?: boolean } = $props();
  let open = $state(false);
  const identifier = $props.id();
  const label = $derived(toolActivityLabel(activity.call));
  const language = $derived.by(() => {
    const path = plainFileReadPath(activity.call);
    return path ? fileLanguage(path) : 'text';
  });
</script>

<section
  class="assistant-section tool-activity flex min-w-0 flex-col"
  role="group"
  aria-label={`Tool ${activity.call.Name}`}
  data-call-id={activity.call.ID}
>
  <button
    type="button"
    class="activity-toggle w-full shrink-0"
    aria-expanded={open}
    aria-controls={identifier}
    onclick={() => (open = !open)}
  >
    <ChevronRight
      class="size-3.5 shrink-0 text-tool-call-accent transition-transform {open ? 'rotate-90' : ''}"
    /><span
      class="min-w-0 flex-1 truncate font-mono font-medium"
      title={`${activity.call.Name}: ${label}`}>{label}</span
    >
    <span class="shrink-0 text-[10px] text-muted-foreground"
      >{activity.result ? 'Result' : running ? 'Waiting' : 'No result'}</span
    >
  </button>
  {#if open}
    <div id={identifier} class="min-w-0 space-y-3 pt-2 pl-3 pb-2">
      {#if unmatched}<p class="mt-2 text-xs text-muted-foreground">
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
      <section aria-label="Tool output" class="min-w-0 pt-2">
        <h3 class="eyebrow">Output</h3>
        {#if activity.result?.Content?.length}<MessageContent
            content={activity.result.Content}
            mode={language === 'markdown' ? 'markdown' : 'code'}
            {language}
            embedded
          />
        {:else}<p class="mt-2 text-xs leading-5 text-muted-foreground">
            {activity.result
              ? 'No output.'
              : running
                ? 'Waiting for the tool result…'
                : 'No result was recorded for this call.'}
          </p>{/if}
      </section>
      <p class="mt-2 break-all font-mono text-[10px] text-muted-foreground">
        Call {activity.call.ID}
      </p>
    </div>
  {/if}
</section>
