<script lang="ts">
  import { ChevronDown } from 'lucide-svelte';
  import type { Model } from '../atoms/types';
  import {
    groupModelOptions,
    modelOption,
    modelOptionMatches,
    type ModelOption,
  } from '../atoms/models';
  import { cn } from '../atoms/utils';
  import { Button } from '../atoms/ui/button';
  import * as Popover from '../atoms/ui/popover';
  import * as Command from '../atoms/ui/command';

  let {
    id,
    models,
    value = $bindable(''),
    disabled = false,
    required = false,
    placeholder = 'Choose a model',
    emptyLabel,
    ariaLabel,
    title,
    class: className,
    onValueChange,
  }: {
    id: string;
    models: Model[];
    value?: string;
    disabled?: boolean;
    required?: boolean;
    placeholder?: string;
    emptyLabel?: string;
    ariaLabel?: string;
    title?: string;
    class?: string;
    onValueChange?: (value: string) => void;
  } = $props();

  let open = $state(false);
  let query = $state('');
  let highlighted = $state('');
  let trigger = $state<HTMLButtonElement | null>(null);
  let searchInput = $state<HTMLInputElement | null>(null);
  let restoreFocus = $state(false);

  const options = $derived.by(() => {
    const catalog = models.map(modelOption);
    if (value && !catalog.some((option) => option.value === value)) {
      catalog.push({ ...modelOption({ ID: value }), disabled: true });
    }
    return catalog;
  });
  const defaultOption = $derived<ModelOption | undefined>(
    emptyLabel === undefined
      ? undefined
      : {
          value: '',
          label: emptyLabel,
          provider: '',
          providerLabel: '',
        },
  );
  const selected = $derived(
    value === '' ? defaultOption : options.find((option) => option.value === value),
  );
  const groups = $derived(groupModelOptions(options, query));
  const showDefault = $derived(defaultOption && modelOptionMatches(defaultOption, query));
  const unavailable = $derived(disabled || (!options.length && !defaultOption));

  function changeOpen(nextOpen: boolean) {
    if (nextOpen && unavailable) return;
    open = nextOpen;
    if (nextOpen) {
      query = '';
      highlighted = selected && !selected.disabled ? `model:${selected.value}` : '';
    }
  }

  function chooseModel(option: ModelOption) {
    if (unavailable || option.disabled) return;
    restoreFocus = true;
    open = false;
    if (option.value === value) return;
    // API-controlled choices keep the confirmed value until their owner accepts
    // it. Local drafts can use bind:value, including the empty default choice.
    if (onValueChange) onValueChange(option.value);
    else value = option.value;
  }

  // A save can disable the trigger during popover focus restoration. Return
  // focus when it completes, unless the user has already navigated elsewhere.
  $effect(() => {
    if (!restoreFocus) return;
    if (!unavailable && !open) {
      trigger?.focus();
      restoreFocus = false;
      return;
    }
    const cancelRestoration = () => {
      restoreFocus = false;
    };
    document.addEventListener('pointerdown', cancelRestoration, true);
    document.addEventListener('keydown', cancelRestoration, true);
    return () => {
      document.removeEventListener('pointerdown', cancelRestoration, true);
      document.removeEventListener('keydown', cancelRestoration, true);
    };
  });
</script>

<Popover.Root bind:open={() => open, changeOpen}>
  <Popover.Trigger bind:ref={trigger}>
    {#snippet child({ props })}
      <Button
        {...props}
        {id}
        type="button"
        variant="outline"
        role="combobox"
        aria-label={ariaLabel}
        aria-required={required || undefined}
        aria-expanded={open}
        {title}
        disabled={unavailable}
        data-value={value}
        class={cn(
          'h-11 min-h-11 w-full min-w-0 justify-between gap-2 px-3 text-left font-normal',
          className,
        )}
        onkeydown={(event: KeyboardEvent) => {
          if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
            event.preventDefault();
            changeOpen(true);
          }
        }}
      >
        <span class="min-w-0 truncate">{selected?.label || placeholder}</span>
        {#if selected?.disabled}<span class="shrink-0 text-xs text-muted-foreground"
            >Unavailable</span
          >{/if}
        <ChevronDown class="size-4 shrink-0 text-muted-foreground" />
      </Button>
    {/snippet}
  </Popover.Trigger>
  <Popover.Content
    align="start"
    class="z-[70] w-(--bits-popover-anchor-width) min-w-0 max-w-[calc(100vw-1.5rem)] gap-0 p-0"
    onOpenAutoFocus={(event) => {
      event.preventDefault();
      searchInput?.focus();
    }}
    onCloseAutoFocus={(event) => {
      if (restoreFocus && unavailable) event.preventDefault();
    }}
  >
    <Command.Root
      shouldFilter={false}
      loop
      bind:value={highlighted}
      class="min-w-0 [&_[data-slot=input-group]]:h-11!"
    >
      <Command.Input
        bind:ref={searchInput}
        bind:value={query}
        placeholder="Search models or providers…"
        aria-label="Search models"
      />
      <Command.List
        aria-label="Available models"
        class="max-h-[min(20rem,calc(var(--bits-popover-content-available-height)-3.5rem))] min-w-0 overflow-x-hidden overscroll-contain"
      >
        {#if !groups.length && !showDefault}
          <div role="status" class="px-3 py-6 text-center text-sm text-muted-foreground">
            No models found.
          </div>
        {/if}
        {#if defaultOption && showDefault}
          <Command.Group>
            <Command.Item
              value="model:"
              data-option-value=""
              data-checked={value === ''}
              onSelect={() => chooseModel(defaultOption!)}
              class="min-h-11 cursor-pointer py-2"
              ><span class="min-w-0 whitespace-normal [overflow-wrap:anywhere]"
                >{defaultOption.label}</span
              ></Command.Item
            >
          </Command.Group>
        {/if}
        {#each groups as group (group.provider)}
          <Command.Group heading={group.label} aria-label={group.label}>
            {#each group.options as option (option.value)}
              <Command.Item
                value={`model:${option.value}`}
                data-option-value={option.value}
                data-checked={value === option.value}
                disabled={option.disabled}
                aria-disabled={option.disabled || undefined}
                onSelect={() => chooseModel(option)}
                class="min-h-11 cursor-pointer py-2"
              >
                <span class="min-w-0 whitespace-normal [overflow-wrap:anywhere]"
                  >{option.label}</span
                >
                {#if option.disabled}<span class="shrink-0 text-xs text-muted-foreground"
                    >Unavailable</span
                  >{/if}
              </Command.Item>
            {/each}
          </Command.Group>
        {/each}
      </Command.List>
    </Command.Root>
  </Popover.Content>
</Popover.Root>
