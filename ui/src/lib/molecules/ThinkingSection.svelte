<script lang="ts">
  import { ChevronRight } from 'lucide-svelte';
  import ActivityIndicator from '../atoms/ActivityIndicator.svelte';
  import { thinkingStatus } from '../atoms/thinking-status';
  import RichText from './content/RichText.svelte';
  let { text, streaming = false }: { text: string; streaming?: boolean } = $props();
  let open = $state(false);
  const identifier = $props.id();
  const status = $derived(streaming ? thinkingStatus(text) : '');
</script>

<section aria-label="Thinking" class="assistant-section">
  <button
    type="button"
    class="activity-toggle w-full"
    aria-expanded={open}
    aria-controls={identifier}
    onclick={() => (open = !open)}
  >
    <ChevronRight class="size-3.5 shrink-0 transition-transform {open ? 'rotate-90' : ''}" />
    {#if streaming}<ActivityIndicator class="mr-1 size-3.5" />{/if}
    <span class="shrink-0">Thinking{streaming ? '…' : ''}</span>
    {#if status}<span data-slot="thinking-status" class="min-w-0 truncate" title={status}
        >· {status}</span
      >{/if}
  </button>
  {#if open}
    <div id={identifier} class="min-w-0 pt-2 pl-3 pb-2 text-muted-foreground">
      <RichText {text} embedded />
    </div>
  {/if}
</section>
