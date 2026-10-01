<script lang="ts">
  import { KeyRound, LoaderCircle, ExternalLink, Unplug } from 'lucide-svelte';
  import { Button } from '$lib/atoms/ui/button';
  import { Input } from '$lib/atoms/ui/input';
  import { Label } from '$lib/atoms/ui/label';
  import type { Model, Provider } from '$lib/atoms/types';
  import ProviderModelList from './ProviderModelList.svelte';

  let {
    provider,
    title,
    keyURL,
    busy = false,
    error = '',
    models = [],
    modelsLoading = false,
    modelsError = '',
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

<section
  aria-label={`${title} API setup`}
  class="flex min-w-0 flex-col rounded-xl border border-border bg-card p-5 sm:p-6"
>
  <div class="mb-5 flex items-start justify-between gap-3">
    <div>
      <KeyRound class="mb-4 size-5 text-muted-foreground" />
      <h2 class="text-base font-medium">{title}</h2>
      <p class="mt-1 text-xs text-muted-foreground">API key · usage-based billing</p>
    </div>
    <span class="rounded-full border border-border px-2.5 py-1 text-[11px] text-muted-foreground"
      >{provider.Connected ? 'Configured' : 'Not connected'}</span
    >
  </div>
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
  {#if error}<p role="alert" class="mt-3 break-words text-sm leading-6 text-destructive">
      {error}
    </p>{/if}
  <div class="mt-4 flex flex-wrap items-center gap-2">
    <Button
      variant="outline"
      class="h-11 text-xs"
      disabled={busy || !provider.Connected}
      onclick={onRefresh}>Refresh models</Button
    >
    {#if provider.Connected}<Button
        variant="ghost"
        class="h-11 text-xs"
        disabled={busy}
        onclick={onDisconnect}><Unplug class="size-3.5" />Disconnect</Button
      >{/if}
  </div>
  <ProviderModelList
    providerID={provider.Name}
    {title}
    {models}
    loading={modelsLoading}
    error={modelsError}
  />
  <a
    href={keyURL}
    target="_blank"
    rel="noopener noreferrer"
    class="mt-2 flex min-h-11 items-center gap-1.5 text-xs text-muted-foreground underline-offset-4 hover:text-foreground hover:underline"
    >Get a {title} API key<ExternalLink class="size-3" /></a
  >
</section>
