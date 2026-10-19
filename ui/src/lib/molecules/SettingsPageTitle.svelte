<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { Cable } from 'lucide-svelte';

  let {
    title,
    description,
    icon: Icon,
    actions,
  }: {
    title: string;
    description: string;
    icon: typeof Cable;
    actions?: Snippet;
  } = $props();
</script>

<header data-settings-page-title class="flex min-w-0 items-start gap-3">
  <Icon aria-hidden="true" class="mt-1 size-5 shrink-0 text-muted-foreground" />
  <div class="min-w-0 flex-1">
    <h2 class="text-lg font-medium">{title}</h2>
    <p class="mt-1 text-sm leading-5 text-muted-foreground">{description}</p>
  </div>
  {#if actions}<div class="shrink-0">{@render actions()}</div>{/if}
</header>
