<script lang="ts">
  import { tick } from 'svelte';
  import { MediaQuery } from 'svelte/reactivity';
  import {
    Cable,
    ChartNoAxesCombined,
    PlugZap,
    SlidersHorizontal,
    Repeat2,
    Bot,
    Eye,
  } from 'lucide-svelte';
  import * as Dialog from '../atoms/ui/dialog';
  import { Button } from '../atoms/ui/button';
  import * as Tabs from '../atoms/ui/tabs';
  import type { SettingsSection } from '../atoms/settings';
  import type { Instance, Session } from '../atoms/types';
  import type { HarnessApi } from '../molecules/api/client';
  import ConnectionForm from '../molecules/ConnectionForm.svelte';
  import ProvidersPanel from './ProvidersPanel.svelte';
  import UsagePanel from './UsagePanel.svelte';
  import RuntimeSettingsPanel from './RuntimeSettingsPanel.svelte';
  import RepetitionPanel from './RepetitionPanel.svelte';
  import SidekickPanel from './SidekickPanel.svelte';
  import WatcherPanel from './WatcherPanel.svelte';
  import SettingsPageTitle from '../molecules/SettingsPageTitle.svelte';

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
    { id: 'instructions' as const, label: 'Instructions', icon: Repeat2 },
    { id: 'sidekick' as const, label: 'Sidekick', icon: Bot },
    { id: 'watcher' as const, label: 'Watchers', icon: Eye },
    { id: 'usage' as const, label: 'Usage', icon: ChartNoAxesCombined },
    { id: 'providers' as const, label: 'Providers', icon: PlugZap },
    { id: 'connection' as const, label: 'Connection', icon: Cable },
  ];
  const sidebar = new MediaQuery('(min-width: 640px)');
  let content = $state<HTMLDivElement>();
  $effect(() => {
    const selectedSection = section;
    const scroller = content;
    void tick().then(() => {
      if (section === selectedSection && content === scroller && scroller) scroller.scrollTop = 0;
    });
  });
</script>

{#snippet connectionRequired()}
  <div class="min-w-0 w-full space-y-5">
    <SettingsPageTitle
      icon={Cable}
      title="Connect your harness"
      description="Connect to the API to view usage, configure providers, and change runtime settings."
    />
    <Button class="h-11" onclick={() => (section = 'connection')}>Open connection settings</Button>
  </div>
{/snippet}

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
    <Tabs.Root
      bind:value={section}
      orientation={sidebar.current ? 'vertical' : 'horizontal'}
      class="min-h-0 min-w-0 flex-1 gap-0"
    >
      <nav
        aria-label="Settings sections"
        class="shrink-0 border-b border-border bg-muted/20 sm:w-44 sm:border-r sm:border-b-0"
      >
        <Tabs.List
          aria-label="Settings pages"
          class="grid h-auto w-full min-w-0 grid-cols-2 gap-1 rounded-none bg-transparent p-2 data-[orientation=horizontal]:h-auto min-[380px]:grid-cols-3 sm:flex sm:flex-col sm:items-stretch sm:justify-start sm:p-3"
        >
          {#each sections as item (item.id)}
            <Tabs.Trigger
              value={item.id}
              class="h-11 min-h-11 min-w-0 shrink-0 gap-2 px-2 text-xs sm:flex-none sm:justify-start sm:px-3"
              aria-current={section === item.id ? 'page' : undefined}
            >
              <item.icon class="hidden size-4 shrink-0 sm:block" /><span class="min-w-0 truncate"
                >{item.label}</span
              >
            </Tabs.Trigger>
          {/each}
        </Tabs.List>
      </nav>
      <div
        bind:this={content}
        data-settings-content
        class="min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-y-contain p-5 sm:p-7"
      >
        <Tabs.Content value="connection" class="m-0 min-w-0">
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
          {/if}
        </Tabs.Content>
        {#key api}
          <Tabs.Content value="general" class="m-0 min-w-0"
            >{#if section === 'general'}{#if api}<RuntimeSettingsPanel
                  {api}
                  {instance}
                />{:else}{@render connectionRequired()}{/if}{/if}</Tabs.Content
          >
          <Tabs.Content value="instructions" class="m-0 min-w-0"
            >{#if section === 'instructions'}{#if api}<RepetitionPanel
                  {api}
                  {instance}
                />{:else}{@render connectionRequired()}{/if}{/if}</Tabs.Content
          >
          <Tabs.Content value="sidekick" class="m-0 min-w-0"
            >{#if section === 'sidekick'}{#if api}<SidekickPanel
                  {api}
                  {instance}
                />{:else}{@render connectionRequired()}{/if}{/if}</Tabs.Content
          >
          <Tabs.Content value="watcher" class="m-0 min-w-0"
            >{#if section === 'watcher'}{#if api}<WatcherPanel
                  {api}
                  {instance}
                  {session}
                />{:else}{@render connectionRequired()}{/if}{/if}</Tabs.Content
          >
          <Tabs.Content value="usage" class="m-0 min-w-0"
            >{#if section === 'usage'}{#if api}<UsagePanel
                  {api}
                  {instance}
                  {session}
                />{:else}{@render connectionRequired()}{/if}{/if}</Tabs.Content
          >
          <Tabs.Content value="providers" class="m-0 min-w-0"
            >{#if section === 'providers'}{#if api}<ProvidersPanel
                  {api}
                  onChanged={onProvidersChanged}
                />{:else}{@render connectionRequired()}{/if}{/if}</Tabs.Content
          >
        {/key}
      </div>
    </Tabs.Root>
  </Dialog.Content>
</Dialog.Root>
