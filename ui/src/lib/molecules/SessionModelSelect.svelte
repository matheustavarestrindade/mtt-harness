<script lang="ts">
  import type { Model } from '../atoms/types';
  import { modelLabel } from '../atoms/format';
  import SelectField from './SelectField.svelte';
  let {
    models,
    value,
    disabled = false,
    onChange,
  }: {
    models: Model[];
    value: string;
    disabled?: boolean;
    onChange: (model: string) => void;
  } = $props();
  const identifier = $props.id();
  const options = $derived([
    ...models.map((model) => ({ value: model.ID, label: modelLabel(model) })),
    ...(value && !models.some((model) => model.ID === value)
      ? [{ value, label: `${value} · unavailable`, disabled: true }]
      : []),
  ]);
</script>

<div class="min-w-0 space-y-2">
  <label for={identifier} class="text-sm font-medium">Model</label>
  <SelectField
    id={identifier}
    ariaLabel="Session model"
    {value}
    {options}
    {disabled}
    title="Change the model for the next request. A smaller context requires confirmation."
    onValueChange={onChange}
  />
</div>
