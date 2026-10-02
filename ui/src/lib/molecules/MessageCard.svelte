<script lang="ts">
  import { Bot, User, Copy, Terminal } from 'lucide-svelte';
  import { toast } from 'svelte-sonner';
  import { Button } from '$lib/atoms/ui/button';
  import type { Message } from '$lib/atoms/types';
  import type { ToolActivity } from '$lib/atoms/transcript';
  import { messageText, formatTimestamp } from '$lib/atoms/format';
  import { copyToClipboard } from './clipboard';
  import MessageContent from './content/MessageContent.svelte';
  import ToolActivitySection from './ToolActivitySection.svelte';
  let {
    message,
    tools = [],
    running = false,
  }: { message: Message; tools?: ToolActivity[]; running?: boolean } = $props();
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
    class="min-w-0 {message.Role === 'user'
      ? 'rounded-2xl border border-border/70 bg-card p-4 sm:p-5'
      : 'py-3'}"
    aria-label={`${message.Role} message`}
  >
    <header class="mb-3 flex items-center gap-2.5">
      <span
        class="grid size-7 shrink-0 place-items-center rounded-lg {message.Role === 'user'
          ? 'bg-secondary text-muted-foreground'
          : 'bg-primary/12 text-primary'}"
        >{#if message.Role === 'user'}<User
            class="size-3.5"
          />{:else if message.Role === 'runtime'}<Terminal class="size-3.5" />{:else}<Bot
            class="size-4"
          />{/if}</span
      ><span class="text-xs font-semibold"
        >{message.Role === 'user'
          ? 'You'
          : message.Role === 'runtime'
            ? 'Runtime'
            : message.Role === 'system'
              ? 'System'
              : 'Assistant'}</span
      ><time class="text-[10px] text-muted-foreground" datetime={message.CreatedAt}
        >{formatTimestamp(message.CreatedAt)}</time
      ><Button
        variant="ghost"
        size="icon"
        class="icon-button ml-auto text-muted-foreground"
        onclick={copyText}
        aria-label="Copy message"
        title="Copy message"><Copy class="size-3.5" /></Button
      >
    </header>
    <MessageContent
      content={message.Content}
      mode={message.Role === 'user' || message.Role === 'runtime' ? 'plain' : 'markdown'}
    />
    {#each tools as activity (activity.call.ID)}<ToolActivitySection {activity} {running} />{/each}
  </article>
{/if}
