<script lang="ts">
  import { ChevronRight } from 'lucide-svelte';
  import RichText from './content/RichText.svelte';
  let { text, streaming = false }: { text: string; streaming?: boolean } = $props();
  let open = $state(false);
  const identifier = $props.id();
</script>

<section aria-label="Thinking" class="assistant-section">
  <button
    type="button"
    class="activity-toggle"
    aria-expanded={open}
    aria-controls={identifier}
    onclick={() => (open = !open)}
  >
    <ChevronRight class="size-3.5 shrink-0 transition-transform {open ? 'rotate-90' : ''}" />
    Thinking{streaming ? '…' : ''}
  </button>
  {#if open}
    <div id={identifier} class="min-w-0 pt-2 pl-3 pb-2 text-muted-foreground">
      <RichText {text} embedded />
    </div>
  {/if}
</section>
