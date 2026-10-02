<script lang="ts">
  import { ChevronRight } from 'lucide-svelte';
  import RichText from './content/RichText.svelte';
  let { text, streaming = false }: { text: string; streaming?: boolean } = $props();
  let open = $state(false);
  const identifier = $props.id();
</script>

<section aria-label="Thinking" class="min-w-0">
  <button
    type="button"
    class="flex min-h-11 items-center gap-1.5 text-left text-xs text-muted-foreground hover:text-foreground focus-visible:outline-2 focus-visible:outline-ring pointer-fine:min-h-8"
    aria-expanded={open}
    aria-controls={identifier}
    onclick={() => (open = !open)}
  >
    <ChevronRight class="size-3.5 shrink-0 transition-transform {open ? 'rotate-90' : ''}" />
    Thinking{streaming ? '…' : ''}
  </button>
  {#if open}
    <div id={identifier} class="min-w-0 border-l border-border pl-3 text-muted-foreground">
      <RichText {text} embedded />
    </div>
  {/if}
</section>
