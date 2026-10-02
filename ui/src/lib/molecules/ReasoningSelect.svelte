<script lang="ts">
  import type { Model } from '../atoms/types';
  import { reasoningEffortLabel } from '../atoms/reasoning';
  let {
    model,
    value = '',
    disabled = false,
    onChange,
  }: {
    model: Model | undefined;
    value?: string;
    disabled?: boolean;
    onChange: (effort: string) => void;
  } = $props();
  const identifier = $props.id();
  const efforts = $derived(model?.ReasoningEfforts ?? []);
</script>

{#if model?.Reasoning || value}
  <div class="flex min-w-0 items-center gap-2 text-xs">
    <label for={identifier} class="shrink-0 text-muted-foreground">Thinking</label>
    <select
      id={identifier}
      aria-label="Thinking effort"
      class="h-11 min-w-0 max-w-full rounded-md border border-input bg-background px-2 text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
      {value}
      disabled={disabled || !model}
      title="Applies to the next model request. Default uses the model's setting."
      onchange={(event) => {
        const effort = event.currentTarget.value;
        event.currentTarget.value = value;
        onChange(effort);
      }}
    >
      <option value=""
        >Default{model?.DefaultReasoningEffort
          ? ` (${reasoningEffortLabel(model.DefaultReasoningEffort)})`
          : ''}</option
      >
      {#each efforts as effort (effort)}<option value={effort}
          >{reasoningEffortLabel(effort)}</option
        >{/each}
      {#if value && !efforts.includes(value)}<option {value} disabled>Unavailable: {value}</option
        >{/if}
    </select>
  </div>
{/if}
