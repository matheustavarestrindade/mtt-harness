<script lang="ts">
  import { FolderPlus, LoaderCircle } from 'lucide-svelte';
  import * as Dialog from '$lib/atoms/ui/dialog';
  import { Input } from '$lib/atoms/ui/input';
  import { Label } from '$lib/atoms/ui/label';
  import { Button } from '$lib/atoms/ui/button';
  import type { InstanceInput, Model } from '$lib/atoms/types';
  import ModelCombobox from './ModelCombobox.svelte';
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
  let workspace = $state('');
  const homeShortcut = $derived(workspace.trim().startsWith('~'));
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
        if (busy || homeShortcut || !workspace.trim() || !model.trim()) return;
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
          aria-invalid={homeShortcut || undefined}
          aria-describedby={homeShortcut ? 'workspace-path-error' : undefined}
        />
        {#if homeShortcut}<p
            id="workspace-path-error"
            role="alert"
            class="text-xs text-destructive"
          >
            Enter the full folder path on the server. Home shortcuts (~) are not expanded.
          </p>{/if}
      </div>
      <div class="space-y-2">
        <Label for="workspace-model">Default model</Label>{#if models.length}<ModelCombobox
            id="workspace-model"
            bind:value={model}
            {models}
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
              : 'The model catalog is unavailable. Enter a model ID from your provider.')}
        </p>
      </div>
      {#if error}<p role="alert" class="break-words text-sm leading-6 text-destructive">
          {error}
        </p>{/if}
      <Dialog.Footer
        ><Button
          class="h-11"
          type="submit"
          disabled={busy || homeShortcut || !workspace.trim() || !model.trim()}
          >{#if busy}<LoaderCircle class="size-4 animate-spin" />{:else}<FolderPlus
              class="size-4"
            />{/if}Create workspace</Button
        ></Dialog.Footer
      >
    </form>
  </Dialog.Content></Dialog.Root
>
