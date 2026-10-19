<script lang="ts">
  import type { Model } from '../atoms/types';
  import ModelCombobox from './ModelCombobox.svelte';
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
</script>

<div class="min-w-0 space-y-2">
  <label for={identifier} class="text-sm font-medium">Model</label>
  <ModelCombobox
    id={identifier}
    ariaLabel="Session model"
    {value}
    {models}
    {disabled}
    title="Change the model for the next request. A smaller context requires confirmation."
    onValueChange={onChange}
  />
</div>
