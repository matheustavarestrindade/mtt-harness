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

<header
  data-settings-page-title
  class={`grid min-w-0 items-start gap-x-3 gap-y-1 ${actions ? 'grid-cols-[20px_minmax(0,1fr)_auto]' : 'grid-cols-[20px_minmax(0,1fr)]'}`}
>
  <Icon aria-hidden="true" class="col-start-1 row-start-1 mt-1 size-5 text-muted-foreground" />
  <h2 class="col-start-2 row-start-1 min-w-0 text-lg font-medium">{title}</h2>
  <p class="col-span-2 row-start-2 min-w-0 text-sm leading-5 text-muted-foreground">
    {description}
  </p>
  {#if actions}<div class="col-start-3 row-span-2 row-start-1">{@render actions()}</div>{/if}
</header>
