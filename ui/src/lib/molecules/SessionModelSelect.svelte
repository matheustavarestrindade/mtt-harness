<script lang="ts">
  import type { Model } from '../atoms/types';
  import { modelLabel } from '../atoms/format';
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
  <select
    id={identifier}
    aria-label="Session model"
    {value}
    {disabled}
    class="field-select h-11 min-w-0 w-full"
    title="Change the model for the next request. A smaller context requires confirmation."
    onchange={(event) => {
      const selected = event.currentTarget.value;
      event.currentTarget.value = value;
      onChange(selected);
    }}
  >
    {#each models as model (model.ID)}<option value={model.ID}>{modelLabel(model)}</option>{/each}
    {#if value && !models.some((model) => model.ID === value)}<option {value} disabled
        >{value} · unavailable</option
      >{/if}
  </select>
</div>
