<script lang="ts">
  import { X } from 'lucide-svelte';
  import * as Dialog from '../atoms/ui/dialog';
  import { Button } from '../atoms/ui/button';
  let { title, closeLabel = 'Close' }: { title: string; closeLabel?: string } = $props();
</script>

<div data-dialog-title-bar class="flex min-h-11 w-full items-center justify-between gap-3">
  <Dialog.Title class="min-w-0 text-base">{title}</Dialog.Title>
  <Dialog.Close
    >{#snippet child({ props })}<Button
        {...props}
        variant="ghost"
        size="icon"
        class="icon-button shrink-0"
        aria-label={closeLabel}><X class="size-4" /></Button
      >{/snippet}</Dialog.Close
  >
</div>
