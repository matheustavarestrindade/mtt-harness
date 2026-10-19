<script lang="ts">
  import { X, LoaderCircle } from 'lucide-svelte';
  import { Button } from '../atoms/ui/button';
  import { shortID } from '../atoms/format';
  import type { QueuedMessageView } from '../atoms/queued-messages';
  import MessageContent from './content/MessageContent.svelte';
  let {
    entry,
    cancelling = false,
    onCancel,
  }: {
    entry: QueuedMessageView;
    cancelling?: boolean;
    onCancel: (identifier: string) => void;
  } = $props();
</script>

<article
  class="chat-message"
  class:user-message={entry.message?.Role === 'user'}
  aria-label={entry.message?.Role === 'user' ? 'user message' : 'Queued message'}
  data-queued-message={entry.id}
>
  <div
    class="mb-1 flex min-h-7 items-center justify-between gap-2 text-[10px] text-muted-foreground"
  >
    <span role="status">Queued</span>
    <Button
      variant="ghost"
      size="icon"
      class="size-7 shrink-0 pointer-coarse:size-11"
      aria-label={`Cancel queued message ${shortID(entry.id)}`}
      title="Cancel queued message"
      disabled={cancelling}
      onclick={() => onCancel(entry.id)}
    >
      {#if cancelling}<LoaderCircle
          class="size-3.5 animate-spin motion-reduce:animate-none"
        />{:else}<X class="size-3.5" />{/if}
    </Button>
  </div>
  {#if entry.message}<MessageContent content={entry.message.Content} mode="plain" />{:else}<p
      class="text-xs text-muted-foreground"
    >
      Message {shortID(entry.id)}
    </p>{/if}
</article>
