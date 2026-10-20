<script lang="ts">
  import { Settings, SlidersHorizontal, FolderCog, BrainCircuit, ChevronUp } from 'lucide-svelte';
  import * as DropdownMenu from '../atoms/ui/dropdown-menu';
  import { buttonVariants } from '../atoms/ui/button';
  import type { SettingsScope } from '../atoms/settings';

  let {
    variant = 'icon',
    workspaceAvailable = false,
    connectionLabel = '',
    connected = false,
    onSettings,
    onSessionSettings,
  }: {
    variant?: 'icon' | 'toolbar' | 'row' | 'footer';
    workspaceAvailable?: boolean;
    connectionLabel?: string;
    connected?: boolean;
    onSettings: (scope: SettingsScope, trigger: HTMLButtonElement | null) => void;
    onSessionSettings?: (trigger: HTMLButtonElement | null) => void;
  } = $props();
  let trigger = $state<HTMLButtonElement | null>(null);
  let selectionStarted = false;
</script>

<DropdownMenu.Root
  onOpenChange={(open) => {
    if (open) selectionStarted = false;
  }}
>
  <DropdownMenu.Trigger
    bind:ref={trigger}
    aria-label="Configuration"
    title="Configuration"
    class={buttonVariants({
      variant: 'ghost',
      class:
        variant === 'footer'
          ? 'h-auto min-h-16 w-full justify-start gap-3 rounded-none border-0 border-t border-border px-5 text-left'
          : variant === 'row'
            ? 'h-11 w-full justify-start gap-3 px-3 text-sm'
            : variant === 'toolbar'
              ? 'icon-button gap-2 px-2 text-xs sm:px-3'
              : 'icon-button text-muted-foreground',
    })}
  >
    {#if variant === 'toolbar'}
      <span
        class={`hidden size-1.5 rounded-full sm:block ${connected ? 'bg-primary' : 'bg-muted-foreground'}`}
      ></span>
      <span class="hidden sm:inline">{connectionLabel}</span><Settings class="size-4" />
    {:else if variant === 'footer'}
      <span class="relative grid size-8 shrink-0 place-items-center rounded-full bg-secondary"
        ><Settings class="size-4 text-muted-foreground" /><span
          class={`absolute right-0 bottom-0 size-2 rounded-full ring-2 ring-card ${connected ? 'bg-primary' : 'bg-muted-foreground'}`}
        ></span></span
      >
      <span class="min-w-0 flex-1"
        ><span class="block text-xs font-medium">Configuration</span><span
          class="mt-0.5 block text-[10px] text-muted-foreground"
          >{connected ? 'Harness connected' : 'API connection required'}</span
        ></span
      ><ChevronUp class="size-3 shrink-0 text-muted-foreground" />
    {:else}<Settings class="size-4" />{#if variant === 'row'}Configuration{/if}{/if}
  </DropdownMenu.Trigger>
  <DropdownMenu.Content
    aria-label="Configuration menu"
    side={variant === 'toolbar' ? 'bottom' : 'top'}
    align={variant === 'footer' || variant === 'row' ? 'start' : 'end'}
    sideOffset={6}
    class="z-[70] min-w-56 max-w-[calc(100vw-1rem)]"
    onCloseAutoFocus={(event) => {
      if (selectionStarted) event.preventDefault();
    }}
  >
    <DropdownMenu.Item
      class="min-h-11 gap-2"
      onSelect={() => {
        selectionStarted = true;
        onSettings('harness', trigger);
      }}><SlidersHorizontal class="size-4" />Settings</DropdownMenu.Item
    >
    <DropdownMenu.Item
      class="min-h-11 gap-2"
      disabled={!workspaceAvailable}
      onSelect={() => {
        selectionStarted = true;
        onSettings('workspace', trigger);
      }}><FolderCog class="size-4" />Workspace settings</DropdownMenu.Item
    >
    {#if onSessionSettings}<DropdownMenu.Separator /><DropdownMenu.Item
        class="min-h-11 gap-2"
        onSelect={() => {
          selectionStarted = true;
          onSessionSettings?.(trigger);
        }}><BrainCircuit class="size-4" />Model and thinking</DropdownMenu.Item
      >{/if}
  </DropdownMenu.Content>
</DropdownMenu.Root>
