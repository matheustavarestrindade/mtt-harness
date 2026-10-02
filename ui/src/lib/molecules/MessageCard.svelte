<script lang="ts">
  import { Copy } from 'lucide-svelte';
  import { toast } from 'svelte-sonner';
  import { Button } from '$lib/atoms/ui/button';
  import type { Message } from '$lib/atoms/types';
  import type { ToolActivity } from '$lib/atoms/transcript';
  import { messageText, formatTimestamp } from '$lib/atoms/format';
  import { copyToClipboard } from './clipboard';
  import MessageContent from './content/MessageContent.svelte';
  import ToolActivitySection from './ToolActivitySection.svelte';
  import ThinkingSection from './ThinkingSection.svelte';
  let {
    message,
    tools = [],
    running = false,
    streaming = false,
  }: {
    message: Message;
    tools?: ToolActivity[];
    running?: boolean;
    streaming?: boolean;
  } = $props();
  const text = $derived(messageText(message));
  async function copyText() {
    try {
      await copyToClipboard(text);
      toast.success('Copied message');
    } catch {
      toast.error('Clipboard access is unavailable. Select the text to copy it.');
    }
  }
</script>

{#if message.Role === 'tool'}
  <ToolActivitySection
    activity={{
      call: { ID: message.ToolCallID, Name: 'Tool result', Input: null },
      result: message,
    }}
    unmatched
  />
{:else}
  <article
    class="chat-message {message.Role === 'user' ? 'user-message' : ''}"
    aria-label={`${message.Role} message`}
  >
    {#if message.Role === 'runtime' || message.Role === 'system'}
      <p class="mb-1 text-[10px] font-medium text-muted-foreground">
        {message.Role === 'runtime' ? 'Runtime' : 'System'}
      </p>
    {/if}
    <div class="message-actions">
      <time class="text-[10px] text-muted-foreground" datetime={message.CreatedAt}
        >{formatTimestamp(message.CreatedAt)}</time
      ><Button
        variant="ghost"
        size="icon"
        class="icon-button text-muted-foreground"
        onclick={copyText}
        aria-label="Copy message"
        title="Copy message"><Copy class="size-3.5" /></Button
      >
    </div>
    <div class="flex min-w-0 flex-col gap-1">
      {#if message.Role === 'assistant' && message.Reasoning}<ThinkingSection
          text={message.Reasoning}
          {streaming}
        />{/if}
      <MessageContent
        content={message.Content}
        mode={message.Role === 'user' || message.Role === 'runtime' ? 'plain' : 'markdown'}
      />
      {#each tools as activity (activity.call.ID)}<ToolActivitySection
          {activity}
          {running}
        />{/each}
    </div>
  </article>
{/if}
