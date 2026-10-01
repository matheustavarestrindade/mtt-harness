<script lang="ts">
  import type { Model } from '$lib/atoms/types';

  let {
    providerID,
    title,
    models = [],
    loading = false,
    error = '',
  }: {
    providerID: string;
    title: string;
    models?: Model[];
    loading?: boolean;
    error?: string;
  } = $props();
</script>

<section aria-label={`${title} models`} aria-busy={loading} class="mt-4 space-y-2">
  <div class="flex items-center gap-2 text-xs text-muted-foreground">
    <h3 class="font-medium">Models</h3>
    {#if !loading && !error}<span>{models.length}</span>{/if}
  </div>
  {#if loading}
    <p role="status" class="text-xs text-muted-foreground">Loading model names…</p>
  {:else if error}
    <p role="alert" class="break-words text-xs leading-5 text-destructive">
      Could not load model names: {error}
    </p>
  {:else if models.length === 0}
    <p class="text-xs text-muted-foreground">No models in the catalog.</p>
  {:else}
    <!-- svelte-ignore a11y_no_noninteractive_tabindex (The scrollable region needs a tab stop for keyboard scrolling.) -->
    <div
      role="region"
      aria-label={`${title} model names`}
      tabindex="0"
      class="max-h-60 overflow-y-auto overscroll-contain rounded-md border border-border outline-offset-2 focus-visible:outline-2 focus-visible:outline-ring"
    >
      <ul class="divide-y divide-border">
        {#each models as model}
          <li class="px-3 py-2">
            {#if model.Name}<span class="mb-1 block break-words text-xs font-medium"
                >{model.Name}</span
              >{/if}
            <code class="block break-all text-xs leading-5">{providerID}/{model.ID}</code>
          </li>
        {/each}
      </ul>
    </div>
  {/if}
</section>
