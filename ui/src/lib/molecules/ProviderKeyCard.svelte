<script lang="ts">
  import { LoaderCircle, ExternalLink, Unplug } from 'lucide-svelte';
  import { Button } from '$lib/atoms/ui/button';
  import { Input } from '$lib/atoms/ui/input';
  import { Label } from '$lib/atoms/ui/label';
  import type { Model, Provider } from '$lib/atoms/types';
  import ProviderCard from './ProviderCard.svelte';

  let {
    provider,
    title,
    keyURL,
    busy = false,
    error = '',
    models = [],
    modelsLoading = false,
    modelsError = '',
    expanded = $bindable<'setup' | 'models' | null>(null),
    onModels,
    onSave,
    onDisconnect,
    onRefresh,
  }: {
    provider: Provider;
    title: string;
    keyURL: string;
    busy?: boolean;
    error?: string;
    models?: Model[];
    modelsLoading?: boolean;
    modelsError?: string;
    expanded?: 'setup' | 'models' | null;
    onModels: () => void;
    onSave: (key: string) => Promise<boolean>;
    onDisconnect: () => void;
    onRefresh: () => void;
  } = $props();
  let key = $state('');
  async function submitProviderKey(event: SubmitEvent) {
    event.preventDefault();
    const submitted = key.trim();
    if (await onSave(submitted)) key = '';
  }
</script>

<ProviderCard
  {provider}
  {title}
  label={`${title} API setup`}
  caption="API key"
  bind:expanded
  {busy}
  {error}
  {models}
  {modelsLoading}
  {modelsError}
  {onModels}
  {onRefresh}
>
  {#snippet setup()}
    <form class="space-y-3" onsubmit={submitProviderKey}>
      <Label for={`provider-key-${provider.Name}`}>{title} API key</Label>
      <Input
        id={`provider-key-${provider.Name}`}
        bind:value={key}
        type="password"
        autocomplete="off"
        spellcheck={false}
        placeholder={provider.Connected ? 'Enter a replacement key' : 'Paste your API key'}
        class="h-11 font-mono text-sm"
        disabled={busy}
        required
      />
      <Button type="submit" class="h-11 w-full" disabled={busy || !key.trim()}
        >{#if busy}<LoaderCircle class="size-4 animate-spin" />{/if}{provider.Connected
          ? 'Replace key'
          : 'Save API key'}</Button
      >
    </form>
    <div class="mt-4 flex flex-wrap items-center gap-2">
      {#if provider.Connected}<Button
          variant="ghost"
          class="h-11 text-xs"
          disabled={busy}
          onclick={onDisconnect}><Unplug class="size-3.5" />Disconnect</Button
        >{/if}
    </div>
    <a
      href={keyURL}
      target="_blank"
      rel="noopener noreferrer"
      class="mt-2 flex min-h-11 items-center gap-1.5 text-xs text-muted-foreground underline-offset-4 hover:text-foreground hover:underline"
      >Get a {title} API key<ExternalLink class="size-3" /></a
    >
  {/snippet}
</ProviderCard>
