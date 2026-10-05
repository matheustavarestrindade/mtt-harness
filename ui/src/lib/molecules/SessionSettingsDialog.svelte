<script lang="ts">
  import type { Model } from '../atoms/types';
  import * as Dialog from '../atoms/ui/dialog';
  import { Button } from '../atoms/ui/button';
  import SessionModelSelect from './SessionModelSelect.svelte';
  import ReasoningSelect from './ReasoningSelect.svelte';
  import ModelChangeConfirmation from './ModelChangeConfirmation.svelte';
  let {
    open = $bindable(false),
    models,
    model,
    modelID,
    effort,
    busy = false,
    disabled = false,
    error = '',
    notice = '',
    pendingModel = null,
    onModelChange,
    onEffortChange,
    onConfirm,
    onCancel,
  }: {
    open?: boolean;
    models: Model[];
    model: Model | undefined;
    modelID: string;
    effort: string;
    busy?: boolean;
    disabled?: boolean;
    error?: string;
    notice?: string;
    pendingModel?: { model: string; currentContext: number; targetContext: number } | null;
    onModelChange: (model: string) => void;
    onEffortChange: (effort: string) => void;
    onConfirm: () => void;
    onCancel: () => void;
  } = $props();
</script>

<Dialog.Root bind:open>
  <Dialog.Content class="max-h-[90dvh] overflow-y-auto sm:max-w-md">
    {#if pendingModel}
      <ModelChangeConfirmation {...pendingModel} {busy} {onConfirm} {onCancel} />
    {:else}
      <Dialog.Header>
        <Dialog.Title>Session settings</Dialog.Title>
        <Dialog.Description
          >Changes apply to the next model request in this conversation.</Dialog.Description
        >
      </Dialog.Header>
      <div class="min-w-0 space-y-5 py-2">
        <div class="space-y-2">
          <SessionModelSelect
            {models}
            value={modelID}
            disabled={disabled || busy}
            onChange={onModelChange}
          />
          <p class="text-xs text-muted-foreground">
            Context window: {model?.ContextMax
              ? `${model.ContextMax.toLocaleString()} tokens`
              : 'unavailable'}
          </p>
        </div>
        {#if model?.Reasoning || effort}
          <ReasoningSelect
            {model}
            value={effort}
            stacked
            disabled={disabled || busy}
            onChange={onEffortChange}
          />
        {:else}
          <p class="text-sm text-muted-foreground">Thinking is not configurable for this model.</p>
        {/if}
      </div>
      <Dialog.Footer class="flex-row items-center justify-between gap-3 sm:justify-between">
        <span role="status" aria-live="polite" class="min-w-0 text-xs text-muted-foreground"
          >{busy ? 'Saving…' : notice || 'Changes are saved automatically.'}</span
        >
        <Button class="h-11 shrink-0" onclick={() => (open = false)}>Done</Button>
      </Dialog.Footer>
    {/if}
    {#if error}<p role="alert" class="break-words text-sm text-destructive">{error}</p>{/if}
  </Dialog.Content>
</Dialog.Root>
