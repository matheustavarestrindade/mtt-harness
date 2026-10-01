<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { ExternalLink, LoaderCircle, RefreshCw, LogIn, Unplug } from 'lucide-svelte';
  import { toast } from 'svelte-sonner';
  import { Button } from '$lib/atoms/ui/button';
  import { Input } from '$lib/atoms/ui/input';
  import { Label } from '$lib/atoms/ui/label';
  import type { DeviceLogin, Model, Provider } from '$lib/atoms/types';
  import type { HarnessApi } from '$lib/molecules/api/client';
  import ProviderKeyCard from '$lib/molecules/ProviderKeyCard.svelte';
  import ProviderModelList from '$lib/molecules/ProviderModelList.svelte';

  let { api, onChanged }: { api: HarnessApi; onChanged: () => Promise<void> } = $props();
  let providers = $state<Provider[]>([]);
  let catalogs = $state<Record<string, { models: Model[]; error: string }>>({});
  let loading = $state(true);
  let error = $state('');
  let busy = $state('');
  let errors = $state<Record<string, string>>({});
  let login = $state<DeviceLogin | null>(null);
  const lifetime = new AbortController();
  let catalogRequest: AbortController | undefined;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const codingPlan = $derived(providers.find((provider) => provider.Authentication === 'chatgpt'));
  const keyProviders = $derived(
    providers.filter((provider) => provider.Authentication === 'api_key'),
  );
  const titleOf = (provider: Provider) =>
    provider.Name === 'openai'
      ? 'OpenAI'
      : provider.Name === 'deepseek'
        ? 'DeepSeek'
        : provider.Name;
  const keyURL = (provider: Provider) =>
    provider.Name === 'deepseek'
      ? 'https://platform.deepseek.com/api_keys'
      : 'https://platform.openai.com/api-keys';
  const messageOf = (failure: unknown) =>
    failure instanceof Error ? failure.message : 'Provider setup failed.';

  async function load() {
    catalogRequest?.abort();
    catalogRequest = new AbortController();
    const signal = AbortSignal.any([lifetime.signal, catalogRequest.signal]);
    loading = true;
    error = '';
    try {
      const result = await api.providers(signal);
      const modelLists = await Promise.all(
        result.map(async (provider) => {
          try {
            return {
              provider: provider.Name,
              models: await api.providerModels(provider.Name, signal),
              error: '',
            };
          } catch (failure) {
            return { provider: provider.Name, models: [], error: messageOf(failure) };
          }
        }),
      );
      if (signal.aborted) return;
      providers = result;
      catalogs = Object.fromEntries(
        modelLists.map(({ provider, models, error }) => [provider, { models, error }]),
      );
    } catch (failure) {
      if (!signal.aborted) error = messageOf(failure);
    } finally {
      if (!signal.aborted) loading = false;
    }
  }
  async function changed() {
    await load();
    if (!lifetime.signal.aborted) await onChanged();
  }
  async function save(provider: Provider, key: string): Promise<boolean> {
    busy = provider.Name;
    errors[provider.Name] = '';
    let saved = false;
    try {
      await api.saveProviderKey(provider.Name, key, lifetime.signal);
      saved = true;
      try {
        await api.refreshProvider(provider.Name, lifetime.signal);
        if (!lifetime.signal.aborted) toast.success(`${titleOf(provider)} is ready`);
      } catch (failure) {
        if (!lifetime.signal.aborted)
          errors[provider.Name] = `Key saved, but model refresh failed: ${messageOf(failure)}`;
      }
      await changed();
    } catch (failure) {
      if (!lifetime.signal.aborted) errors[provider.Name] = messageOf(failure);
    } finally {
      if (!lifetime.signal.aborted) busy = '';
    }
    return saved;
  }
  async function action(identifier: string, operation: () => Promise<unknown>) {
    busy = identifier;
    errors[identifier] = '';
    try {
      await operation();
      await changed();
    } catch (failure) {
      if (!lifetime.signal.aborted) errors[identifier] = messageOf(failure);
    } finally {
      if (!lifetime.signal.aborted) busy = '';
    }
  }
  async function poll(identifier: string) {
    const providerID = login?.Provider;
    if (!providerID) return;
    try {
      const result = await api.providerLogin(providerID, identifier, lifetime.signal);
      if (lifetime.signal.aborted || login?.ID !== identifier) return;
      login = result;
      if (result.Status === 'pending') {
        timer = setTimeout(() => void poll(identifier), 2000);
        return;
      }
      if (result.Status === 'connected') {
        toast.success('OpenAI coding plan connected');
        try {
          await api.refreshProvider(providerID, lifetime.signal);
        } catch (failure) {
          if (!lifetime.signal.aborted)
            errors[providerID] = `Signed in, but model refresh failed: ${messageOf(failure)}`;
        }
        await changed();
      }
    } catch (failure) {
      if (!lifetime.signal.aborted) errors[providerID] = messageOf(failure);
    }
  }
  async function signIn() {
    const providerID = codingPlan?.Name;
    if (!providerID) return;
    busy = providerID;
    errors[busy] = '';
    clearTimeout(timer);
    try {
      const result = await api.startProviderLogin(providerID, lifetime.signal);
      if (lifetime.signal.aborted) return;
      login = result;
      timer = setTimeout(() => void poll(result.ID), 2000);
    } catch (failure) {
      if (!lifetime.signal.aborted) errors[providerID] = messageOf(failure);
    } finally {
      if (!lifetime.signal.aborted) busy = '';
    }
  }
  async function cancelLogin() {
    if (!login) return;
    clearTimeout(timer);
    const identifier = login.ID;
    const providerID = login.Provider;
    await action(providerID, () =>
      api.cancelProviderLogin(providerID, identifier, lifetime.signal),
    );
    if (!lifetime.signal.aborted && login?.ID === identifier) login = null;
  }
  onMount(() => {
    void load();
  });
  onDestroy(() => {
    lifetime.abort();
    clearTimeout(timer);
  });
</script>

<section
  aria-label="Provider setup"
  class="min-h-0 flex-1 overflow-y-auto overscroll-y-contain px-4 py-6 sm:px-8 sm:py-8"
>
  <div class="mx-auto max-w-5xl">
    <div class="mb-7 flex items-start justify-between gap-4">
      <div>
        <h1 class="text-2xl font-medium tracking-tight">Providers</h1>
        <p class="mt-2 max-w-xl text-sm leading-6 text-muted-foreground">
          Connect OpenAI or DeepSeek. Keys and account credentials stay in the harness database, not
          in this browser.
        </p>
      </div>
      <Button
        variant="ghost"
        size="icon"
        class="icon-button shrink-0"
        aria-label="Reload providers"
        disabled={!!busy || loading}
        onclick={() => void load()}><RefreshCw class="size-4" /></Button
      >
    </div>
    {#if loading}<p role="status" class="flex items-center gap-2 text-sm text-muted-foreground">
        <LoaderCircle class="size-4 animate-spin" />Loading providers…
      </p>{/if}
    {#if error}<p role="alert" class="mb-4 break-words text-sm text-destructive">{error}</p>{/if}
    <div class="grid items-start gap-4 xl:grid-cols-2">
      {#each keyProviders as provider (provider.Name)}
        <ProviderKeyCard
          {provider}
          title={titleOf(provider)}
          keyURL={keyURL(provider)}
          busy={!!busy}
          error={errors[provider.Name] ?? ''}
          models={catalogs[provider.Name]?.models ?? []}
          modelsLoading={loading}
          modelsError={catalogs[provider.Name]?.error ?? ''}
          onSave={(key) => save(provider, key)}
          onDisconnect={() =>
            void action(provider.Name, () => api.deleteProviderKey(provider.Name, lifetime.signal))}
          onRefresh={() =>
            void action(provider.Name, () => api.refreshProvider(provider.Name, lifetime.signal))}
        />
      {/each}
      {#if codingPlan}
        <section
          aria-label="OpenAI coding plan setup"
          class="min-w-0 rounded-xl border border-border bg-card p-5 sm:p-6"
        >
          <div class="mb-5 flex items-start justify-between gap-3">
            <div>
              <LogIn class="mb-4 size-5 text-muted-foreground" />
              <h2 class="text-base font-medium">OpenAI coding plan</h2>
              <p class="mt-1 text-xs text-muted-foreground">
                Sign in with ChatGPT · subscription access
              </p>
            </div>
            <span
              class="rounded-full border border-border px-2.5 py-1 text-[11px] text-muted-foreground"
              >{codingPlan.Connected ? 'Configured' : 'Not connected'}</span
            >
          </div>
          <p class="mb-4 text-sm leading-6 text-muted-foreground">
            Use your ChatGPT account for Codex access. This is separate from a usage-billed OpenAI
            API key.
          </p>
          {#if login?.Status === 'pending'}
            <div class="space-y-3 rounded-lg border border-border bg-background/40 p-4">
              <Label for="device-user-code">One-time sign-in code</Label><Input
                id="device-user-code"
                value={login.UserCode}
                readonly
                class="h-12 text-center font-mono text-lg tracking-widest"
                onclick={(event) => event.currentTarget.select()}
              />
              <a
                href={login.VerificationURL}
                target="_blank"
                rel="noopener noreferrer"
                class="flex min-h-11 items-center justify-center gap-2 rounded-md bg-primary px-3 text-sm font-medium text-primary-foreground"
                >Open OpenAI sign-in<ExternalLink class="size-4" /></a
              >
              <p class="text-xs leading-5 text-muted-foreground">
                Sign in and enter this code. Enable device-code login in your ChatGPT security
                settings if requested. The code expires in 15 minutes.
              </p>
              <p role="status" class="flex items-center gap-2 text-xs text-muted-foreground">
                <LoaderCircle class="size-3.5 animate-spin" />Waiting for OpenAI approval…
              </p>
              <Button
                variant="outline"
                class="h-11 w-full"
                disabled={!!busy}
                onclick={() => void cancelLogin()}>Cancel sign-in</Button
              >
            </div>
          {:else}
            {#if login?.Error}<p role="alert" class="mb-3 break-words text-sm text-destructive">
                {login.Error}
              </p>{/if}
            <Button class="h-11 w-full" disabled={!!busy} onclick={() => void signIn()}
              >{#if busy === codingPlan.Name}<LoaderCircle
                  class="size-4 animate-spin"
                />{:else}<LogIn class="size-4" />{/if}{codingPlan.Connected
                ? 'Sign in again'
                : 'Sign in with ChatGPT'}</Button
            >
          {/if}
          {#if errors[codingPlan.Name]}<p
              role="alert"
              class="mt-3 break-words text-sm leading-6 text-destructive"
            >
              {errors[codingPlan.Name]}
            </p>{/if}
          {#if codingPlan.Connected}<Button
              variant="ghost"
              class="mt-3 h-11 text-xs"
              disabled={!!busy}
              onclick={() =>
                void action(codingPlan.Name, async () => {
                  clearTimeout(timer);
                  login = null;
                  await api.disconnectCodingPlan(codingPlan.Name, lifetime.signal);
                })}><Unplug class="size-3.5" />Disconnect coding plan</Button
            >{/if}
          <p class="mt-4 text-xs text-muted-foreground">
            Models use the <code>{codingPlan.Name}/</code> prefix. Plan limits still apply.
          </p>
          <ProviderModelList
            providerID={codingPlan.Name}
            title="OpenAI coding plan"
            models={catalogs[codingPlan.Name]?.models ?? []}
            {loading}
            error={catalogs[codingPlan.Name]?.error ?? ''}
          />
        </section>
      {/if}
    </div>
    {#if providers.some((provider) => provider.Name === 'test')}<p
        class="mt-6 text-xs leading-5 text-muted-foreground"
      >
        The test provider is also available for connection checks. It does not use a real AI model.
      </p>
      <ProviderModelList
        providerID="test"
        title="Test provider"
        models={catalogs.test?.models ?? []}
        {loading}
        error={catalogs.test?.error ?? ''}
      />
    {/if}
  </div>
</section>
