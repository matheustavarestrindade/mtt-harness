<script lang="ts">
  import { MessageSquarePlus, LoaderCircle } from 'lucide-svelte';
  import * as Dialog from '$lib/atoms/ui/dialog';
  import { Label } from '$lib/atoms/ui/label';
  import { Button } from '$lib/atoms/ui/button';
  import type { Model } from '$lib/atoms/types';
  import { modelLabel } from '$lib/atoms/format';
  import ReasoningSelect from './ReasoningSelect.svelte';
  import ModelPricing from './ModelPricing.svelte';
  import SelectField from './SelectField.svelte';
  let {
    open = $bindable(false),
    models = [],
    defaultModel = '',
    busy = false,
    onCreate,
  }: {
    open?: boolean;
    models?: Model[];
    defaultModel?: string;
    busy?: boolean;
    onCreate: (model: string, effort: string) => Promise<void>;
  } = $props();
  let selectedModel = $state('');
  let reasoningEffort = $state('');
  const model = $derived(models.find((model) => model.ID === selectedModel));
  $effect(() => {
    if (open)
      selectedModel =
        models.find((model) => model.ID === defaultModel || model.ID.endsWith(`/${defaultModel}`))
          ?.ID ??
        models[0]?.ID ??
        '';
  });
  $effect(() => {
    selectedModel;
    open;
    reasoningEffort = '';
  });
</script>

<Dialog.Root bind:open
  ><Dialog.Content class="max-h-[90dvh] overflow-y-auto">
    <Dialog.Header
      ><Dialog.Title>Start a session</Dialog.Title><Dialog.Description
        >A fresh conversation in the current workspace.</Dialog.Description
      ></Dialog.Header
    >
    <form
      class="space-y-5"
      onsubmit={(event) => {
        event.preventDefault();
        void onCreate(selectedModel, reasoningEffort);
      }}
    >
      <div class="space-y-2">
        <Label for="session-model">Model</Label><SelectField
          id="session-model"
          bind:value={selectedModel}
          options={models.map((model) => ({
            value: model.ID,
            label: `${modelLabel(model)}${model.Tools ? '' : ' · tool support not declared'}`,
          }))}
          disabled={busy}
          required
        />{#if !models.length}<p class="text-sm text-muted-foreground">
            No available models. Check the provider configuration and this instance's model list.
          </p>{/if}
      </div>
      <ReasoningSelect
        {model}
        value={reasoningEffort}
        disabled={busy}
        onChange={(value) => (reasoningEffort = value)}
      />
      {#if model}<ModelPricing {model} />{/if}
      <Dialog.Footer
        ><Button class="h-11" type="submit" disabled={busy || !selectedModel}
          >{#if busy}<LoaderCircle class="size-4 animate-spin" />{:else}<MessageSquarePlus
              class="size-4"
            />{/if}Create session</Button
        ></Dialog.Footer
      >
    </form>
  </Dialog.Content></Dialog.Root
>
