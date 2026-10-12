<script lang="ts">
  import type { Snippet } from 'svelte';
  import { ChevronDown, List, Settings2, RefreshCw } from 'lucide-svelte';
  import { Button } from '../atoms/ui/button';
  import type { Model, Provider } from '../atoms/types';
  import ProviderModelList from './ProviderModelList.svelte';

  let {
    provider,
    title,
    label,
    caption,
    setup,
    setupLabel = 'Configure',
    expanded = $bindable<'setup' | 'models' | null>(null),
    busy = false,
    error = '',
    models = [],
    modelsLoading = false,
    modelsError = '',
    onModels,
    onRefresh,
  }: {
    provider: Provider;
    title: string;
    label: string;
    caption: string;
    setup?: Snippet;
    setupLabel?: string;
    expanded?: 'setup' | 'models' | null;
    busy?: boolean;
    error?: string;
    models?: Model[];
    modelsLoading?: boolean;
    modelsError?: string;
    onModels: () => void;
    onRefresh: () => void;
  } = $props();
  const identifier = $props.id();
  function toggleModels() {
    expanded = expanded === 'models' ? null : 'models';
    if (expanded === 'models') onModels();
  }
</script>

<section aria-label={label} class="min-w-0 rounded-lg border border-border bg-card p-4">
  <h3 class="truncate text-sm font-medium" {title}>{title}</h3>
  <p class="mt-1 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
    <span>{caption}</span><span aria-hidden="true">·</span>
    <span class="inline-flex items-center gap-1.5">
      <span
        aria-hidden="true"
        class={provider.Connected
          ? 'size-1.5 rounded-full bg-foreground'
          : 'size-1.5 rounded-full bg-muted-foreground/40'}
      ></span>
      {provider.Connected ? 'Configured' : 'Not connected'}
    </span>
  </p>
  <div class="mt-4 flex min-w-0 gap-2">
    {#if setup}
      <Button
        variant={expanded === 'setup' ? 'secondary' : 'outline'}
        class="h-11 min-w-0 flex-1 gap-1.5 px-2 text-xs"
        disabled={busy}
        aria-expanded={expanded === 'setup'}
        aria-controls={`${identifier}-setup`}
        onclick={() => (expanded = expanded === 'setup' ? null : 'setup')}
      >
        <Settings2 class="size-3.5 shrink-0" /><span class="truncate">{setupLabel}</span>
      </Button>
    {/if}
    <Button
      variant={expanded === 'models' ? 'secondary' : 'outline'}
      class="h-11 min-w-0 flex-1 gap-1.5 px-2 text-xs"
      disabled={busy}
      aria-expanded={expanded === 'models'}
      aria-controls={`${identifier}-models`}
      onclick={toggleModels}
    >
      <List class="size-3.5 shrink-0" /><span>Models</span>
      {#if provider.ModelCount !== undefined}<span class="text-muted-foreground"
          >{provider.ModelCount}</span
        >{/if}
      <ChevronDown
        class={expanded === 'models' ? 'size-3 shrink-0 rotate-180' : 'size-3 shrink-0'}
      />
    </Button>
  </div>
  {#if error}<p role="alert" class="mt-3 break-words text-xs leading-5 text-destructive">
      {error}
    </p>{/if}
  {#if expanded === 'setup' && setup}
    <div id={`${identifier}-setup`} class="mt-4 min-w-0 border-t border-border pt-4">
      {@render setup()}
    </div>
  {:else if expanded === 'models'}
    <div id={`${identifier}-models`} class="mt-4 min-w-0 border-t border-border pt-3">
      <Button
        variant="ghost"
        class="h-11 gap-1.5 px-2 text-xs"
        disabled={busy || modelsLoading || !provider.Connected}
        onclick={onRefresh}
      >
        <RefreshCw class="size-3.5" />Refresh models
      </Button>
      <ProviderModelList
        providerID={provider.Name}
        {title}
        {models}
        loading={modelsLoading}
        error={modelsError}
      />
    </div>
  {/if}
</section>
