<script lang="ts">
  import { ChevronRight } from 'lucide-svelte';
  import ActivityIndicator from '../atoms/ActivityIndicator.svelte';
  import { thinkingStatus } from '../atoms/thinking-status';
  import RichText from './content/RichText.svelte';
  import * as Collapsible from '../atoms/ui/collapsible';
  let { text, streaming = false }: { text: string; streaming?: boolean } = $props();
  let open = $state(false);
  const status = $derived(streaming ? thinkingStatus(text) : '');
</script>

<Collapsible.Root bind:open role="region" aria-label="Thinking" class="assistant-section">
  <Collapsible.Trigger class="activity-toggle w-full">
    <ChevronRight class="size-3.5 shrink-0 transition-transform {open ? 'rotate-90' : ''}" />
    {#if streaming}<ActivityIndicator class="mr-1 size-3.5" />{/if}
    <span class="shrink-0">Thinking{streaming ? '…' : ''}</span>
    {#if status}<span data-slot="thinking-status" class="min-w-0 truncate" title={status}
        >· {status}</span
      >{/if}
  </Collapsible.Trigger>
  <Collapsible.Content class="min-w-0 pt-2 pl-3 pb-2 text-muted-foreground">
    {#if open}<RichText {text} embedded />{/if}
  </Collapsible.Content>
</Collapsible.Root>
