<script lang="ts">
  import { ArrowUp, Square, CornerDownLeft, LoaderCircle } from 'lucide-svelte';
  import { Button } from '$lib/atoms/ui/button';
  import { Textarea } from '$lib/atoms/ui/textarea';
  let {
    value = $bindable(''),
    disabled = false,
    sending = false,
    running = false,
    cancelling = false,
    onSend,
    onCancel,
  }: {
    value?: string;
    disabled?: boolean;
    sending?: boolean;
    running?: boolean;
    cancelling?: boolean;
    onSend: () => void;
    onCancel: () => void;
  } = $props();
  function handleComposerKeydown(event: KeyboardEvent) {
    // Enter keeps a newline on mobile keyboards. Desktop users have an explicit
    // modifier shortcut that does not interfere with IME composition.
    if (
      event.key === 'Enter' &&
      (event.metaKey || event.ctrlKey) &&
      !event.isComposing &&
      !disabled &&
      !sending &&
      value.trim()
    ) {
      event.preventDefault();
      onSend();
    }
  }
</script>

<form
  class="rounded-2xl border border-input bg-card shadow-lg shadow-black/10 transition-colors focus-within:border-primary/50"
  onsubmit={(event) => {
    event.preventDefault();
    if (!disabled && !sending && value.trim()) onSend();
  }}
>
  <label for="message-input" class="sr-only">Message</label>
  <Textarea
    id="message-input"
    bind:value
    class="min-h-24 max-h-52 resize-none border-0 bg-transparent p-4 text-base shadow-none focus-visible:ring-0 dark:bg-transparent sm:text-sm"
    placeholder={disabled
      ? 'Choose a session to start a conversation…'
      : running
        ? 'Add another message to the queue…'
        : 'Give the harness a task…'}
    {disabled}
    onkeydown={handleComposerKeydown}
  />
  <div class="flex min-h-14 items-center justify-between gap-2 px-3 pb-3">
    <span class="pl-1 text-[10px] text-muted-foreground"
      >{#if running}<span class="flex items-center gap-1.5"
          ><span class="size-1.5 rounded-full bg-primary"></span>New messages join the queue</span
        >{:else}<span class="flex items-center gap-1.5"
          ><CornerDownLeft class="size-3" /><span class="hidden sm:inline"
            >Ctrl / ⌘ + Enter to send</span
          ><span class="sm:hidden">Enter for a new line</span></span
        >{/if}</span
    >
    <div class="flex items-center gap-2">
      {#if running}<Button
          class="h-11 gap-2"
          variant="outline"
          type="button"
          disabled={cancelling}
          onclick={onCancel}
          ><Square class="size-3 fill-current" />{cancelling ? 'Stopping…' : 'Stop'}</Button
        >{/if}<Button
        type="submit"
        class="size-11 rounded-xl"
        size="icon"
        aria-label={running ? 'Queue message' : 'Send message'}
        disabled={disabled || sending || !value.trim()}
        >{#if sending}<LoaderCircle class="size-4 animate-spin" />{:else}<ArrowUp
            class="size-5"
          />{/if}</Button
      >
    </div>
  </div>
</form>
