<script lang="ts">
  import { LoaderCircle, Trash2 } from 'lucide-svelte';
  import * as Dialog from '$lib/atoms/ui/dialog';
  import { Button } from '$lib/atoms/ui/button';
  import type { Session } from '$lib/atoms/types';
  import { shortID } from '$lib/atoms/format';
  let {
    open = $bindable(false),
    session,
    busy = false,
    error = '',
    onDelete,
  }: {
    open?: boolean;
    session: Session | null;
    busy?: boolean;
    error?: string;
    onDelete: () => void;
  } = $props();
</script>

<Dialog.Root bind:open>
  <Dialog.Content class="max-h-[90dvh] overflow-y-auto sm:max-w-md">
    <Dialog.Header>
      <Dialog.Title>Delete session {session ? shortID(session.ID) : ''}?</Dialog.Title>
      <Dialog.Description
        >Delete this conversation and its child sessions permanently.</Dialog.Description
      >
    </Dialog.Header>
    <p class="text-sm leading-6 text-muted-foreground">
      Saved messages, thinking, and tool output will be removed. Workspace files and recorded usage
      totals stay available.
    </p>
    <p class="text-xs leading-5 text-muted-foreground">
      Active turns, queued messages, or running processes must be stopped before deletion.
    </p>
    {#if error}<p role="alert" class="break-words text-sm text-destructive">{error}</p>{/if}
    <Dialog.Footer>
      <Button
        type="button"
        variant="outline"
        class="h-11"
        disabled={busy}
        onclick={() => (open = false)}>Cancel</Button
      >
      <Button
        type="button"
        variant="destructive"
        class="h-11"
        disabled={busy || !session}
        onclick={onDelete}
      >
        {#if busy}<LoaderCircle class="size-4 animate-spin" />{:else}<Trash2 class="size-4" />{/if}
        {busy ? 'Deleting…' : 'Delete session'}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
