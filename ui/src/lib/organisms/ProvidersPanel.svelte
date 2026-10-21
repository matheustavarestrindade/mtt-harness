<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { ExternalLink, LoaderCircle, RefreshCw, LogIn, Unplug, PlugZap } from 'lucide-svelte';
  import { toast } from 'svelte-sonner';
  import { Button } from '$lib/atoms/ui/button';
  import { Input } from '$lib/atoms/ui/input';
  import { Label } from '$lib/atoms/ui/label';
  import type { DeviceLogin, Model, Provider } from '$lib/atoms/types';
  import type { HarnessApi } from '$lib/molecules/api/client';
  import ProviderKeyCard from '$lib/molecules/ProviderKeyCard.svelte';
  import ProviderCard from '$lib/molecules/ProviderCard.svelte';
  import SettingsPageTitle from '$lib/molecules/SettingsPageTitle.svelte';

  let { api, onChanged }: { api: HarnessApi; onChanged: () => Promise<void> } = $props();
  let providers = $state<Provider[]>([]);
  let catalogs = $state<Record<string, { models: Model[]; error: string; loading?: boolean }>>({});
  let expanded = $state<Record<string, 'setup' | 'models' | null>>({});
  let loading = $state(true);
  let error = $state('');
  let busy = $state('');
  let errors = $state<Record<string, string>>({});
  let login = $state<DeviceLogin | null>(null);
  const lifetime = new AbortController();
  let catalogRequest: AbortController | undefined;
  const modelRequests = new Map<string, AbortController>();
  let timer: ReturnType<typeof setTimeout> | undefined;
  const codingPlan = $derived(providers.find((provider) => provider.Authentication === 'chatgpt'));
  const keyProviders = $derived(
    providers.filter((provider) => provider.Authentication === 'api_key'),
  );
  const providerDisplayName = (provider: Provider) =>
    provider.Name === 'openai'
      ? 'OpenAI'
      : provider.Name === 'deepseek'
        ? 'DeepSeek'
        : provider.Name;
  const providerKeyManagementURL = (provider: Provider) =>
    provider.Name === 'deepseek'
      ? 'https://platform.deepseek.com/api_keys'
      : 'https://platform.openai.com/api-keys';
  const providerErrorMessage = (failure: unknown) =>
    failure instanceof Error ? failure.message : 'Provider setup failed.';

  async function loadProvidersAndModels() {
    catalogRequest?.abort();
    for (const [providerID, request] of modelRequests) {
      request.abort();
      if (catalogs[providerID]?.loading) delete catalogs[providerID];
    }
    modelRequests.clear();
    catalogRequest = new AbortController();
    const signal = AbortSignal.any([lifetime.signal, catalogRequest.signal]);
    loading = true;
    error = '';
    try {
      const result = await api.providers(signal);
      const modelLists = await Promise.all(
        result
          .filter((provider) => expanded[provider.Name] === 'models')
          .map(async (provider) => {
            try {
              return {
                provider: provider.Name,
                models: await api.providerModels(provider.Name, signal),
                error: '',
              };
            } catch (failure) {
              return { provider: provider.Name, models: [], error: providerErrorMessage(failure) };
            }
          }),
      );
      if (signal.aborted) return;
      // Bound disclosures need an explicit value before their cards mount.
      for (const provider of result) expanded[provider.Name] ??= null;
      providers = result;
      catalogs = Object.fromEntries(
        modelLists.map(({ provider, models, error }) => [provider, { models, error }]),
      );
    } catch (failure) {
      if (!signal.aborted) error = providerErrorMessage(failure);
    } finally {
      if (!signal.aborted) loading = false;
    }
  }
  async function loadProviderModels(providerID: string) {
    if (catalogs[providerID] && !catalogs[providerID].error) return;
    modelRequests.get(providerID)?.abort();
    const request = new AbortController();
    modelRequests.set(providerID, request);
    const signal = AbortSignal.any([lifetime.signal, request.signal]);
    catalogs[providerID] = { models: [], error: '', loading: true };
    try {
      const models = await api.providerModels(providerID, signal);
      if (!signal.aborted) catalogs[providerID] = { models, error: '', loading: false };
    } catch (failure) {
      if (!signal.aborted)
        catalogs[providerID] = { models: [], error: providerErrorMessage(failure), loading: false };
    } finally {
      if (modelRequests.get(providerID) === request) modelRequests.delete(providerID);
    }
  }
  async function refreshProviderViews() {
    await loadProvidersAndModels();
    if (!lifetime.signal.aborted) await onChanged();
  }
  async function saveProviderAPIKey(provider: Provider, key: string): Promise<boolean> {
    busy = provider.Name;
    errors[provider.Name] = '';
    let saved = false;
    try {
      await api.saveProviderKey(provider.Name, key, lifetime.signal);
      saved = true;
      try {
        await api.refreshProvider(provider.Name, lifetime.signal);
        if (!lifetime.signal.aborted) toast.success(`${providerDisplayName(provider)} is ready`);
      } catch (failure) {
        if (!lifetime.signal.aborted)
          errors[provider.Name] =
            `Key saved, but model refresh failed: ${providerErrorMessage(failure)}`;
      }
      await refreshProviderViews();
    } catch (failure) {
      if (!lifetime.signal.aborted) errors[provider.Name] = providerErrorMessage(failure);
    } finally {
      if (!lifetime.signal.aborted) busy = '';
    }
    return saved;
  }
  async function runProviderAction(providerID: string, operation: () => Promise<unknown>) {
    busy = providerID;
    errors[providerID] = '';
    try {
      await operation();
      await refreshProviderViews();
    } catch (failure) {
      if (!lifetime.signal.aborted) errors[providerID] = providerErrorMessage(failure);
    } finally {
      if (!lifetime.signal.aborted) busy = '';
    }
  }
  async function pollDeviceLoginStatus(loginID: string) {
    const providerID = login?.Provider;
    if (!providerID) return;
    try {
      const loginStatus = await api.providerLogin(providerID, loginID, lifetime.signal);
      if (lifetime.signal.aborted || login?.ID !== loginID) return;
      login = loginStatus;
      if (loginStatus.Status === 'pending') {
        timer = setTimeout(() => void pollDeviceLoginStatus(loginID), 2000);
        return;
      }
      if (loginStatus.Status === 'connected') {
        toast.success('OpenAI coding plan connected');
        try {
          await api.refreshProvider(providerID, lifetime.signal);
        } catch (failure) {
          if (!lifetime.signal.aborted)
            errors[providerID] =
              `Signed in, but model refresh failed: ${providerErrorMessage(failure)}`;
        }
        await refreshProviderViews();
      }
    } catch (failure) {
      if (!lifetime.signal.aborted) errors[providerID] = providerErrorMessage(failure);
    }
  }
  async function startCodingPlanLogin() {
    const providerID = codingPlan?.Name;
    if (!providerID) return;
    busy = providerID;
    errors[busy] = '';
    clearTimeout(timer);
    try {
      const deviceLogin = await api.startProviderLogin(providerID, lifetime.signal);
      if (lifetime.signal.aborted) return;
      login = deviceLogin;
      timer = setTimeout(() => void pollDeviceLoginStatus(deviceLogin.ID), 2000);
    } catch (failure) {
      if (!lifetime.signal.aborted) errors[providerID] = providerErrorMessage(failure);
    } finally {
      if (!lifetime.signal.aborted) busy = '';
    }
  }
  async function cancelDeviceLogin() {
    if (!login) return;
    clearTimeout(timer);
    const loginID = login.ID;
    const providerID = login.Provider;
    await runProviderAction(providerID, () =>
      api.cancelProviderLogin(providerID, loginID, lifetime.signal),
    );
    if (!lifetime.signal.aborted && login?.ID === loginID) login = null;
  }
  onMount(() => {
    void loadProvidersAndModels();
  });
  onDestroy(() => {
    lifetime.abort();
    clearTimeout(timer);
  });
</script>

<section aria-label="Provider setup" class="min-w-0 w-full space-y-5">
  <SettingsPageTitle
    icon={PlugZap}
    title="Providers"
    description="Connect providers and manage available models."
  >
    {#snippet actions()}
      <Button
        variant="ghost"
        size="icon"
        class="icon-button shrink-0"
        aria-label="Reload providers"
        disabled={!!busy || loading}
        onclick={() => void loadProvidersAndModels()}><RefreshCw class="size-4" /></Button
      >
    {/snippet}
  </SettingsPageTitle>
  {#if loading}<p role="status" class="flex items-center gap-2 text-sm text-muted-foreground">
      <LoaderCircle class="size-4 animate-spin" />Loading providers…
    </p>{/if}
  {#if error}<p role="alert" class="mb-4 break-words text-sm text-destructive">{error}</p>{/if}
  <div
    class="grid min-w-0 items-start gap-3 [grid-template-columns:repeat(auto-fit,minmax(min(100%,15rem),1fr))]"
  >
    {#each keyProviders as provider (provider.Name)}
      <ProviderKeyCard
        {provider}
        title={providerDisplayName(provider)}
        keyURL={providerKeyManagementURL(provider)}
        busy={!!busy || loading}
        error={errors[provider.Name] ?? ''}
        models={catalogs[provider.Name]?.models ?? []}
        modelsLoading={loading || !!catalogs[provider.Name]?.loading}
        modelsError={catalogs[provider.Name]?.error ?? ''}
        bind:expanded={expanded[provider.Name]}
        onModels={() => void loadProviderModels(provider.Name)}
        onSave={(key) => saveProviderAPIKey(provider, key)}
        onDisconnect={() =>
          void runProviderAction(provider.Name, () =>
            api.deleteProviderKey(provider.Name, lifetime.signal),
          )}
        onRefresh={() =>
          void runProviderAction(provider.Name, () =>
            api.refreshProvider(provider.Name, lifetime.signal),
          )}
      />
    {/each}
    {#if codingPlan}
      <ProviderCard
        provider={codingPlan}
        title="OpenAI coding plan"
        label="OpenAI coding plan setup"
        caption="ChatGPT"
        bind:expanded={expanded[codingPlan.Name]}
        busy={!!busy || loading}
        error={errors[codingPlan.Name] ?? ''}
        setupLabel={login?.Status === 'pending' ? 'Sign-in pending' : 'Configure'}
        models={catalogs[codingPlan.Name]?.models ?? []}
        modelsLoading={loading || !!catalogs[codingPlan.Name]?.loading}
        modelsError={catalogs[codingPlan.Name]?.error ?? ''}
        onModels={() => codingPlan && void loadProviderModels(codingPlan.Name)}
        onRefresh={() =>
          codingPlan &&
          void runProviderAction(codingPlan.Name, () =>
            api.refreshProvider(codingPlan!.Name, lifetime.signal),
          )}
      >
        {#snippet setup()}
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
                onclick={() => void cancelDeviceLogin()}>Cancel sign-in</Button
              >
            </div>
          {:else}
            {#if login?.Error}<p role="alert" class="mb-3 break-words text-sm text-destructive">
                {login.Error}
              </p>{/if}
            <Button
              class="h-11 w-full"
              disabled={!!busy}
              onclick={() => void startCodingPlanLogin()}
              >{#if busy === codingPlan.Name}<LoaderCircle
                  class="size-4 animate-spin"
                />{:else}<LogIn class="size-4" />{/if}{codingPlan.Connected
                ? 'Sign in again'
                : 'Sign in with ChatGPT'}</Button
            >
          {/if}
          {#if codingPlan.Connected}<Button
              variant="ghost"
              class="mt-3 h-11 text-xs"
              disabled={!!busy}
              onclick={() =>
                void runProviderAction(codingPlan.Name, async () => {
                  clearTimeout(timer);
                  login = null;
                  await api.disconnectCodingPlan(codingPlan.Name, lifetime.signal);
                })}><Unplug class="size-3.5" />Disconnect coding plan</Button
            >{/if}
        {/snippet}
      </ProviderCard>
    {/if}
    {#each providers.filter((provider) => provider.Authentication !== 'api_key' && provider.Authentication !== 'chatgpt') as provider (provider.Name)}
      <ProviderCard
        {provider}
        title={provider.Name === 'test' ? 'Test provider' : providerDisplayName(provider)}
        label={`${provider.Name === 'test' ? 'Test provider' : providerDisplayName(provider)} setup`}
        caption="No credentials"
        bind:expanded={expanded[provider.Name]}
        busy={!!busy || loading}
        error={errors[provider.Name] ?? ''}
        models={catalogs[provider.Name]?.models ?? []}
        modelsLoading={loading || !!catalogs[provider.Name]?.loading}
        modelsError={catalogs[provider.Name]?.error ?? ''}
        onModels={() => void loadProviderModels(provider.Name)}
        onRefresh={() =>
          void runProviderAction(provider.Name, () =>
            api.refreshProvider(provider.Name, lifetime.signal),
          )}
      />
    {/each}
  </div>
</section>
