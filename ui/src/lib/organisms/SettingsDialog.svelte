<script lang="ts">
  import { Cable, ChartNoAxesCombined, PlugZap, SlidersHorizontal } from 'lucide-svelte';
  import * as Dialog from '../atoms/ui/dialog';
  import { Button } from '../atoms/ui/button';
  import type { SettingsSection } from '../atoms/settings';
  import type { Instance, Session } from '../atoms/types';
  import type { HarnessApi } from '../molecules/api/client';
  import ConnectionForm from '../molecules/ConnectionForm.svelte';
  import ProvidersPanel from './ProvidersPanel.svelte';
  import UsagePanel from './UsagePanel.svelte';
  import RuntimeSettingsPanel from './RuntimeSettingsPanel.svelte';

  let {
    open = $bindable(false),
    section = $bindable<SettingsSection>('general'),
    api,
    instance,
    session,
    initialBase,
    initialToken,
    busy,
    error,
    onConnect,
    onDisconnect,
    onProvidersChanged,
  }: {
    open?: boolean;
    section?: SettingsSection;
    api: HarnessApi | null;
    instance: Instance | null;
    session: Session | null;
    initialBase: string;
    initialToken: string;
    busy: boolean;
    error: string;
    onConnect: (base: string, token: string) => Promise<void>;
    onDisconnect: () => void;
    onProvidersChanged: () => Promise<void>;
  } = $props();
  const sections = [
    { id: 'general' as const, label: 'General', icon: SlidersHorizontal },
    { id: 'usage' as const, label: 'Usage', icon: ChartNoAxesCombined },
    { id: 'providers' as const, label: 'Providers', icon: PlugZap },
    { id: 'connection' as const, label: 'Connection', icon: Cable },
  ];
</script>

<Dialog.Root bind:open>
  <Dialog.Content
    class="settings-dialog flex h-[min(90dvh,780px)] max-w-[calc(100%_-_1rem)] flex-col gap-0 overflow-hidden p-0 sm:max-w-4xl"
  >
    <Dialog.Header class="shrink-0 border-b border-border px-5 py-4 pr-16 text-left">
      <Dialog.Title class="text-base">Settings</Dialog.Title>
      <Dialog.Description class="sr-only"
        >Manage your connection, providers, usage, and runtime limits.</Dialog.Description
      >
    </Dialog.Header>
    <div class="flex min-h-0 min-w-0 flex-1 flex-col sm:flex-row">
      <nav
        aria-label="Settings sections"
        class="flex shrink-0 overflow-x-auto border-b border-border bg-muted/20 p-2 sm:w-44 sm:flex-col sm:gap-1 sm:border-r sm:border-b-0 sm:p-3"
      >
        {#each sections as item (item.id)}
          <Button
            variant={section === item.id ? 'secondary' : 'ghost'}
            class="h-11 shrink-0 gap-2 px-3 text-xs sm:justify-start"
            aria-current={section === item.id ? 'page' : undefined}
            onclick={() => (section = item.id)}
          >
            <item.icon class="size-4 shrink-0" /><span>{item.label}</span>
          </Button>
        {/each}
      </nav>
      <div class="min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-y-contain p-5 sm:p-7">
        {#if section === 'connection'}
          <ConnectionForm
            {initialBase}
            {initialToken}
            {busy}
            {error}
            connected={!!api}
            {onConnect}
            {onDisconnect}
          />
        {:else if !api}
          <div class="mx-auto max-w-xl py-10">
            <h2 class="text-xl font-medium">Connect your harness</h2>
            <p class="mt-3 text-sm leading-6 text-muted-foreground">
              Connect to the API to view usage, configure providers, and change runtime settings.
            </p>
            <Button class="mt-5 h-11" onclick={() => (section = 'connection')}
              >Open connection settings</Button
            >
          </div>
        {:else}
          {#key api}
            {#if section === 'general'}<RuntimeSettingsPanel {api} {instance} />
            {:else if section === 'usage'}<UsagePanel {api} {instance} {session} />
            {:else if section === 'providers'}<ProvidersPanel
                {api}
                onChanged={onProvidersChanged}
              />{/if}
          {/key}
        {/if}
      </div>
    </div>
  </Dialog.Content>
</Dialog.Root>
