<script lang="ts">
  import { X, LoaderCircle, Paperclip } from 'lucide-svelte';
  import { Button } from '../atoms/ui/button';
  import { shortID } from '../atoms/format';
  import type { QueuedMessageView } from '../atoms/queued-messages';
  import MessageContent from './content/MessageContent.svelte';
  let {
    entry,
    cancelling = false,
    onCancel,
    onLoad,
    loading = false,
    error = '',
  }: {
    entry: QueuedMessageView;
    cancelling?: boolean;
    onCancel: (identifier: string) => void;
    onLoad?: (identifier: string) => void;
    loading?: boolean;
    error?: string;
  } = $props();
  const role = $derived(entry.message?.Role ?? entry.preview?.Role);
</script>

<article
  class="chat-message"
  class:user-message={role === 'user'}
  aria-label={role === 'user' ? 'user message' : 'Queued message'}
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
  {#if entry.message}<MessageContent content={entry.message.Content} mode="plain" />
  {:else if entry.preview}
    <div class="space-y-2">
      {#each entry.preview.Content ?? [] as content}
        {#if content.Filename || content.Type !== 'text'}<div
            class="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground"
          >
            <Paperclip class="size-3 shrink-0" /><span class="break-all"
              >{content.Filename || `${content.Type} attachment`}</span
            >
          </div>{/if}
        {#if content.Type === 'text' && content.Text}<p
            class="whitespace-pre-wrap break-words text-sm"
          >
            {content.Text}
          </p>{/if}
      {/each}
      {#if entry.preview.Truncated}
        <div class="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <span>Preview limited</span><Button
            variant="ghost"
            class="h-8 px-2 text-xs pointer-coarse:h-11"
            disabled={loading || !onLoad}
            onclick={() => onLoad?.(entry.id)}
            >{#if loading}<LoaderCircle
                class="size-3 animate-spin motion-reduce:animate-none"
              />{/if}Show full message</Button
          >
        </div>
      {/if}
    </div>
  {:else}<p class="text-xs text-muted-foreground">
      Message {shortID(entry.id)}
    </p>{/if}
  {#if error}<p role="alert" class="mt-2 text-xs text-destructive">{error}</p>{/if}
</article>
