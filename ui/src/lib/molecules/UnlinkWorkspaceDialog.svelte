<script lang="ts">
  import { LoaderCircle, Unlink } from 'lucide-svelte';
  import * as Dialog from '../atoms/ui/dialog';
  import { Button } from '../atoms/ui/button';
  import type { Instance } from '../atoms/types';
  let {
    open = $bindable(false),
    instance,
    busy = false,
    error = '',
    onUnlink,
  }: {
    open?: boolean;
    instance: Instance | null;
    busy?: boolean;
    error?: string;
    onUnlink: () => void;
  } = $props();
</script>

<Dialog.Root bind:open>
  <Dialog.Content class="max-h-[90dvh] overflow-y-auto sm:max-w-md">
    <Dialog.Header>
      <Dialog.Title>Unlink workspace?</Dialog.Title>
      <Dialog.Description
        >Remove this workspace from the harness list. Files and folders stay untouched.</Dialog.Description
      >
    </Dialog.Header>
    <p class="break-all font-mono text-xs">{instance?.Workspace}</p>
    <p class="text-sm text-muted-foreground">Saved chats, memories, and usage records are kept.</p>
    {#if error}<p role="alert" class="break-words text-sm text-destructive">{error}</p>{/if}
    <Dialog.Footer>
      <Button variant="outline" class="h-11" disabled={busy} onclick={() => (open = false)}
        >Cancel</Button
      >
      <Button class="h-11" disabled={busy || !instance} onclick={onUnlink}>
        {#if busy}<LoaderCircle class="size-4 animate-spin" />{:else}<Unlink class="size-4" />{/if}
        {busy ? 'Unlinking…' : 'Unlink workspace'}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
