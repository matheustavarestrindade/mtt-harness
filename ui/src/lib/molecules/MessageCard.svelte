<script lang="ts">
  import { Bot, User, Terminal, Copy, ChevronDown, FileText } from 'lucide-svelte';
  import { toast } from 'svelte-sonner';
  import { Button } from '$lib/atoms/ui/button';
  import type { Message } from '$lib/atoms/types';
  import { messageText, time } from '$lib/atoms/format';
  let { message }: { message: Message } = $props();
  const text = $derived(messageText(message));
  async function copyText() {
    try {
      await navigator.clipboard.writeText(text);
      toast.success('Copied message');
    } catch {
      toast.error('Clipboard access is unavailable. Select the text to copy it.');
    }
  }
</script>

{#if message.Role === 'tool'}
  <details class="group/tool min-w-0 rounded-xl border border-border bg-card/50">
    <summary class="flex min-h-12 cursor-pointer list-none items-center gap-2 px-4 py-3 text-xs"
      ><Terminal class="size-3.5 text-primary" /><span class="font-medium">Tool result</span><span
        class="min-w-0 flex-1 truncate font-mono text-[10px] text-muted-foreground"
        >{message.ToolCallID}</span
      ><ChevronDown
        class="size-3.5 text-muted-foreground transition-transform group-open/tool:rotate-180"
      /></summary
    >
    <div class="min-w-0 border-t border-border p-3">
      <pre class="tool-output whitespace-pre-wrap break-all">{text || 'No text output.'}</pre>
    </div>
  </details>
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
        >{#if message.Role === 'user'}<User class="size-3.5" />{:else}<Bot
            class="size-4"
          />{/if}</span
      ><span class="text-xs font-semibold"
        >{message.Role === 'user'
          ? 'You'
          : message.Role === 'system'
            ? 'System'
            : 'Assistant'}</span
      ><time class="text-[10px] text-muted-foreground" datetime={message.CreatedAt}
        >{time(message.CreatedAt)}</time
      ><Button
        variant="ghost"
        size="icon"
        class="icon-button ml-auto text-muted-foreground"
        onclick={copyText}
        aria-label="Copy message"
        title="Copy message"><Copy class="size-3.5" /></Button
      >
    </header>
    {#if text}<div class="message-copy">{text}</div>{/if}
    {#each message.Content ?? [] as content}
      {#if content.Type !== 'text'}<div
          class="mt-3 flex min-w-0 items-center gap-2 rounded-lg border border-border p-3 text-xs text-muted-foreground"
        >
          <FileText class="size-4 shrink-0" /><span class="truncate"
            >{content.Filename || content.Type} {content.MIME ? `· ${content.MIME}` : ''}</span
          >
        </div>{/if}
    {/each}
    {#each message.ToolCalls ?? [] as call (call.ID)}
      <details class="mt-3 min-w-0 rounded-lg border border-border bg-black/15">
        <summary class="flex min-h-11 cursor-pointer items-center gap-2 px-3 text-xs"
          ><Terminal class="size-3.5 text-primary" /><span class="font-mono">{call.Name}</span><span
            class="ml-auto text-[10px] text-muted-foreground">Arguments</span
          ></summary
        >
        <pre class="tool-output m-2 whitespace-pre-wrap break-all">{JSON.stringify(
            call.Input,
            null,
            2,
          )}</pre>
      </details>
    {/each}
  </article>
{/if}
