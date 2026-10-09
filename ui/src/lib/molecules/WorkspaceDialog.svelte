<script lang="ts">
  import { FolderPlus, LoaderCircle } from 'lucide-svelte';
  import * as Dialog from '$lib/atoms/ui/dialog';
  import { Input } from '$lib/atoms/ui/input';
  import { Label } from '$lib/atoms/ui/label';
  import { Button } from '$lib/atoms/ui/button';
  import type { InstanceInput, Model } from '$lib/atoms/types';
  import { modelLabel } from '$lib/atoms/format';
  import SelectField from './SelectField.svelte';
  let {
    open = $bindable(false),
    models = [],
    busy = false,
    catalogError = '',
    error = '',
    onCreate,
  }: {
    open?: boolean;
    models?: Model[];
    busy?: boolean;
    catalogError?: string;
    error?: string;
    onCreate: (input: InstanceInput) => Promise<void>;
  } = $props();
  let workspace = $state('/workspace');
  let model = $state('');
  $effect(() => {
    if (open && models.length && !models.some((available) => available.ID === model))
      model = models[0].ID;
  });
</script>

<Dialog.Root bind:open
  ><Dialog.Content>
    <Dialog.Header
      ><div class="mb-3 grid size-11 place-items-center rounded-xl bg-primary/10 text-primary">
        <FolderPlus class="size-5" />
      </div>
      <Dialog.Title>Open a workspace</Dialog.Title><Dialog.Description
        >Choose an existing directory on the harness server and its default model.</Dialog.Description
      ></Dialog.Header
    >
    <form
      class="space-y-5"
      onsubmit={(event) => {
        event.preventDefault();
        void onCreate({ workspace: workspace.trim(), default_model: model });
      }}
    >
      <div class="space-y-2">
        <Label for="workspace-path">Project directory</Label><Input
          id="workspace-path"
          class="h-11 font-mono text-sm"
          bind:value={workspace}
          required
          placeholder="/path/to/project"
          disabled={busy}
        />
      </div>
      <div class="space-y-2">
        <Label for="workspace-model">Default model</Label>{#if models.length}<SelectField
            id="workspace-model"
            bind:value={model}
            options={models.map((item) => ({ value: item.ID, label: modelLabel(item) }))}
            required
            disabled={busy}
          />{:else}<Input
            id="workspace-model"
            class="h-11 font-mono text-sm"
            bind:value={model}
            placeholder="provider/model"
            required
            disabled={busy}
          />{/if}
        <p class="text-xs leading-5 text-muted-foreground">
          {catalogError ||
            (models.length
              ? 'Models come from the provider catalog. You can choose a different model for each session.'
              : 'Enter a provider/model ID. For the built-in test provider, use test/test-model.')}
        </p>
      </div>
      {#if error}<p role="alert" class="break-words text-sm leading-6 text-destructive">
          {error}
        </p>{/if}
      <Dialog.Footer
        ><Button class="h-11" type="submit" disabled={busy || !workspace.trim() || !model.trim()}
          >{#if busy}<LoaderCircle class="size-4 animate-spin" />{:else}<FolderPlus
              class="size-4"
            />{/if}Create workspace</Button
        ></Dialog.Footer
      >
    </form>
  </Dialog.Content></Dialog.Root
>
