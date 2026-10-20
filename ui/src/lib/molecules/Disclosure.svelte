<script lang="ts">
  import type { Snippet } from 'svelte';
  import { ChevronRight } from 'lucide-svelte';
  import * as Collapsible from '../atoms/ui/collapsible';

  let {
    open = $bindable(false),
    title,
    heading,
    children,
    label,
    class: className = '',
    triggerClass = 'min-h-7 py-1 text-xs text-muted-foreground pointer-coarse:min-h-11',
    contentClass = '',
  }: {
    open?: boolean;
    title?: string;
    heading?: Snippet;
    children?: Snippet;
    label?: string;
    class?: string;
    triggerClass?: string;
    contentClass?: string;
  } = $props();
</script>

<Collapsible.Root
  bind:open
  class={`min-w-0 ${className}`}
  role={label ? 'group' : undefined}
  aria-label={label}
>
  <Collapsible.Trigger class={`flex min-w-0 w-full items-center gap-2 text-left ${triggerClass}`}>
    <ChevronRight class={`size-3.5 shrink-0 transition-transform ${open ? 'rotate-90' : ''}`} />
    {#if heading}{@render heading()}{:else}<span class="min-w-0 break-words">{title}</span>{/if}
  </Collapsible.Trigger>
  <Collapsible.Content class={`min-w-0 ${contentClass}`}>
    {#if open}{@render children?.()}{/if}
  </Collapsible.Content>
</Collapsible.Root>
