<script lang="ts">
  import * as Select from '../atoms/ui/select';
  import type { SelectOption } from '../atoms/select';

  let {
    id,
    value = $bindable<string[]>([]),
    options,
    disabled = false,
    required = false,
    placeholder = 'Choose options',
    ariaLabel,
    onValueChange,
  }: {
    id: string;
    value?: string[];
    options: SelectOption[];
    disabled?: boolean;
    required?: boolean;
    placeholder?: string;
    ariaLabel?: string;
    onValueChange?: (value: string[]) => void;
  } = $props();

  function chooseValues(selected: string[]) {
    if (onValueChange) onValueChange(selected);
    else value = selected;
  }
</script>

<Select.Root
  type="multiple"
  items={options}
  bind:value={() => value, chooseValues}
  {disabled}
  {required}
>
  <Select.Trigger
    {id}
    aria-label={ariaLabel}
    class="h-11 min-h-11 w-full min-w-0 px-3 text-left *:data-[slot=select-value]:min-w-0 *:data-[slot=select-value]:flex-1 *:data-[slot=select-value]:truncate"
  >
    <Select.Value {placeholder} />
  </Select.Trigger>
  <Select.Content
    class="z-[60] max-h-[min(20rem,var(--bits-select-content-available-height))] w-(--bits-select-anchor-width) min-w-0 max-w-[calc(100vw-1.5rem)]"
  >
    <Select.Group>
      {#each options as option (option.value)}
        <Select.Item
          value={option.value}
          label={option.label}
          disabled={option.disabled}
          data-option-value={option.value}
          class="min-h-11 cursor-pointer py-2 [&>span:last-child]:min-w-0 [&>span:last-child]:whitespace-normal"
          >{option.label}</Select.Item
        >
      {/each}
    </Select.Group>
  </Select.Content>
</Select.Root>
