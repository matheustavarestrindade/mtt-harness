<script lang="ts">
  import type { Model } from '../atoms/types';
  import { reasoningEffortLabel } from '../atoms/reasoning';
  import SelectField from './SelectField.svelte';
  let {
    model,
    value = '',
    disabled = false,
    stacked = false,
    onChange,
  }: {
    model: Model | undefined;
    value?: string;
    disabled?: boolean;
    stacked?: boolean;
    onChange: (effort: string) => void;
  } = $props();
  const identifier = $props.id();
  const efforts = $derived(model?.ReasoningEfforts ?? []);
  const options = $derived([
    {
      value: '',
      label: `Default${model?.DefaultReasoningEffort ? ` (${reasoningEffortLabel(model.DefaultReasoningEffort)})` : ''}`,
    },
    ...efforts.map((effort) => ({ value: effort, label: reasoningEffortLabel(effort) })),
    ...(value && !efforts.includes(value)
      ? [{ value, label: `Unavailable: ${value}`, disabled: true }]
      : []),
  ]);
</script>

{#if model?.Reasoning || value}
  <div class={stacked ? 'min-w-0 space-y-2' : 'flex min-w-0 items-center gap-2 text-xs'}>
    <label
      for={identifier}
      class={stacked ? 'text-sm font-medium' : 'shrink-0 text-muted-foreground'}>Thinking</label
    >
    <SelectField
      id={identifier}
      ariaLabel="Thinking effort"
      class={stacked ? '' : 'flex-1 text-xs'}
      {value}
      {options}
      disabled={disabled || !model}
      title="Applies to the next model request. Default uses the model's setting."
      onValueChange={onChange}
    />
  </div>
{/if}
