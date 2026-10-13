<script lang="ts">
  import { ArrowUp, Square, LoaderCircle, Settings } from 'lucide-svelte';
  import { Button } from '$lib/atoms/ui/button';
  import { Textarea } from '$lib/atoms/ui/textarea';
  import type { Model } from '../atoms/types';
  import { attachmentIssue, type DraftAttachment } from '../atoms/attachments';
  import AttachmentTray from './AttachmentTray.svelte';
  import AttachmentMenu from './AttachmentMenu.svelte';
  let {
    value = $bindable(''),
    disabled = false,
    sending = false,
    running = false,
    cancelling = false,
    onSend,
    onCancel,
    onSettings,
    settingsOpen = false,
    attachments = [],
    attachmentError = '',
    model,
    protocol,
    onFiles,
    onRemoveAttachment,
  }: {
    value?: string;
    disabled?: boolean;
    sending?: boolean;
    running?: boolean;
    cancelling?: boolean;
    onSend: () => void;
    onCancel: () => void;
    onSettings?: () => void;
    settingsOpen?: boolean;
    attachments?: DraftAttachment[];
    attachmentError?: string;
    model?: Model;
    protocol?: string;
    onFiles?: (files: File[]) => void;
    onRemoveAttachment?: (id: string) => void;
  } = $props();
  let textarea = $state<HTMLTextAreaElement | null>(null);
  let dragging = $state(false);
  const blocked = $derived(attachments.some((item) => attachmentIssue(item, model, protocol)));
  const canSend = $derived(
    !disabled && !sending && !blocked && Boolean(value.trim() || attachments.length),
  );
  function chooseFiles(files: File[]) {
    if (!disabled && !sending && files.length) onFiles?.(files);
  }
  function resizeComposer() {
    if (!textarea) return;
    if (!value) {
      textarea.style.height = '44px';
      return;
    }
    textarea.style.height = '0px';
    textarea.style.height = `${Math.min(192, Math.max(44, textarea.scrollHeight))}px`;
  }
  $effect(() => {
    value;
    resizeComposer();
  });
  $effect(() => {
    const element = textarea;
    if (!element) return;
    let previousWidth = element.clientWidth;
    const observer = new ResizeObserver(() => {
      if (element.clientWidth === previousWidth) return;
      previousWidth = element.clientWidth;
      resizeComposer();
    });
    observer.observe(element);
    return () => observer.disconnect();
  });
  function handleComposerKeydown(event: KeyboardEvent) {
    // IME confirmation is text input, not a request to send the draft.
    if (event.key !== 'Enter' || event.isComposing || event.keyCode === 229) return;
    if (event.metaKey || event.ctrlKey) {
      // Command+Enter does not insert a newline consistently across browsers.
      event.preventDefault();
      if (disabled) return;
      const textarea = event.currentTarget as HTMLTextAreaElement;
      textarea.setRangeText('\n', textarea.selectionStart, textarea.selectionEnd, 'end');
      textarea.dispatchEvent(
        new InputEvent('input', {
          bubbles: true,
          inputType: 'insertLineBreak',
          data: '\n',
        }),
      );
      return;
    }
    if (event.shiftKey || event.altKey) return;
    event.preventDefault();
    if (!event.repeat && canSend) onSend();
  }
</script>

<form
  class="flex min-w-0 flex-col gap-1 rounded-xl border border-input bg-card p-1.5 shadow-sm transition-colors focus-within:border-primary/50"
  class:border-primary={dragging}
  ondragover={(event) => {
    if (event.dataTransfer?.types.includes('Files')) {
      event.preventDefault();
      dragging = !disabled;
    }
  }}
  ondragleave={(event) => {
    if (!event.currentTarget.contains(event.relatedTarget as Node | null)) dragging = false;
  }}
  ondrop={(event) => {
    if (event.dataTransfer?.files.length) {
      event.preventDefault();
      dragging = false;
      chooseFiles(Array.from(event.dataTransfer.files));
    }
  }}
  onsubmit={(event) => {
    event.preventDefault();
    if (canSend) onSend();
  }}
>
  {#if attachments.length}<AttachmentTray
      items={attachments}
      {model}
      {protocol}
      disabled={sending}
      onRemove={(id) => onRemoveAttachment?.(id)}
    />{/if}
  {#if attachmentError}<p
      role="alert"
      class="max-h-24 overflow-y-auto whitespace-pre-wrap break-words px-2 text-xs text-destructive"
    >
      {attachmentError}
    </p>{/if}
  <div class="flex w-full min-w-0 items-end gap-1">
    {#if onFiles}
      <AttachmentMenu {model} {protocol} disabled={disabled || sending} onFiles={chooseFiles} />
    {/if}
    <label for="message-input" class="sr-only">Message</label>
    <Textarea
      id="message-input"
      enterkeyhint="send"
      bind:value
      bind:ref={textarea}
      rows={1}
      aria-describedby="composer-hint"
      style="field-sizing: fixed"
      class="min-h-11 max-h-48 min-w-0 flex-1 resize-none overflow-y-auto border-0 bg-transparent px-2.5 py-3 text-base leading-5 shadow-none focus-visible:ring-0 dark:bg-transparent sm:text-sm"
      placeholder={disabled ? 'Choose a session…' : 'Message…'}
      {disabled}
      onkeydown={handleComposerKeydown}
      onpaste={(event) => {
        const files = Array.from(event.clipboardData?.files ?? []);
        if (files.length) {
          event.preventDefault();
          chooseFiles(files);
        }
      }}
    />
    <span id="composer-hint" class="sr-only"
      >Enter to send. Shift or Command + Enter for a new line. {running
        ? 'New messages join the queue.'
        : ''}</span
    >
    <div class="flex shrink-0 items-center gap-0.5">
      {#if onSettings}<Button
          type="button"
          variant="ghost"
          size="icon"
          class="icon-button text-muted-foreground hover:text-foreground"
          aria-label="Session settings"
          title="Session settings"
          aria-haspopup="dialog"
          aria-expanded={settingsOpen}
          onclick={onSettings}><Settings class="size-4" /></Button
        >{/if}
      {#if running}<Button
          class="size-11"
          variant="outline"
          size="icon"
          type="button"
          aria-label="Stop"
          title="Stop generation"
          disabled={cancelling}
          onclick={onCancel}
          >{#if cancelling}<LoaderCircle class="size-4 animate-spin" />{:else}<Square
              class="size-3 fill-current"
            />{/if}</Button
        >{/if}<Button
        type="submit"
        class="size-11 rounded-lg"
        size="icon"
        aria-label={running ? 'Queue message' : 'Send message'}
        disabled={!canSend}
        >{#if sending}<LoaderCircle class="size-4 animate-spin" />{:else}<ArrowUp
            class="size-5"
          />{/if}</Button
      >
    </div>
  </div>
</form>
