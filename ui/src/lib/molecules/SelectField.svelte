<script lang="ts">
  import * as Select from '../atoms/ui/select';
  import { cn } from '../atoms/utils';
  import type { SelectOption } from '../atoms/select';

  let {
    id,
    value = $bindable(''),
    options,
    disabled = false,
    required = false,
    placeholder = 'Choose an option',
    ariaLabel,
    title,
    class: className,
    onValueChange,
  }: {
    id: string;
    value?: string;
    options: SelectOption[];
    disabled?: boolean;
    required?: boolean;
    placeholder?: string;
    ariaLabel?: string;
    title?: string;
    class?: string;
    onValueChange?: (value: string) => void;
  } = $props();
  let open = $state(false);
  let trigger = $state<HTMLButtonElement | null>(null);
  let restoreFocus = $state(false);

  // A saving state can disable the trigger before the menu returns focus.
  // Restore it after the save unless the user has navigated elsewhere.
  $effect(() => {
    if (!restoreFocus) return;
    if (!disabled && !open) {
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

  // Empty is a real API choice for model-default effort. Encoding every value
  // keeps that choice distinct from Select's empty/placeholder state.
  const encodeValue = (value: string) => `option:${value}`;
  const items = $derived(
    options.map((option) => ({ ...option, value: encodeValue(option.value) })),
  );
  const selectedValue = $derived(
    options.some((option) => option.value === value) ? encodeValue(value) : '',
  );
  function chooseValue(encoded: string) {
    const option = options.find((option) => encodeValue(option.value) === encoded);
    if (!option || option.disabled || option.value === value) return;
    // Controlled API selections stay at the confirmed value until their owner
    // accepts the change. Bound draft fields update immediately instead.
    if (onValueChange) {
      restoreFocus = true;
      onValueChange(option.value);
    } else value = option.value;
  }
</script>

<Select.Root
  type="single"
  bind:open
  {items}
  {required}
  disabled={disabled || !options.length}
  bind:value={() => selectedValue, chooseValue}
>
  <Select.Trigger
    bind:ref={trigger}
    {id}
    aria-label={ariaLabel}
    {title}
    data-value={value}
    class={cn(
      'h-11 min-h-11 w-full min-w-0 px-3 text-left *:data-[slot=select-value]:min-w-0 *:data-[slot=select-value]:flex-1 *:data-[slot=select-value]:truncate',
      className,
    )}
  >
    <Select.Value {placeholder} />
  </Select.Trigger>
  <Select.Content
    class="z-[60] max-h-[min(20rem,var(--bits-select-content-available-height))] w-(--bits-select-anchor-width) min-w-0 max-w-[calc(100vw-1.5rem)]"
  >
    <Select.Group>
      {#each options as option (option.value)}
        <Select.Item
          value={encodeValue(option.value)}
          label={option.label}
          disabled={option.disabled}
          aria-disabled={option.disabled || undefined}
          data-option-value={option.value}
          class="min-h-11 cursor-pointer py-2 [&>span:last-child]:min-w-0 [&>span:last-child]:shrink [&>span:last-child]:whitespace-normal"
        >
          <span class="min-w-0 text-left whitespace-normal [overflow-wrap:anywhere]"
            >{option.label}</span
          >
        </Select.Item>
      {/each}
    </Select.Group>
  </Select.Content>
</Select.Root>
