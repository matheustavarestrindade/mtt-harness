<script lang="ts">
  import { FileText } from 'lucide-svelte';
  import { toast } from 'svelte-sonner';
  import { fileLanguage } from '../atoms/content';
  import { dataLabel, structuredJSON } from '../atoms/structured-data';
  import {
    parseWatcherInput,
    watcherConversation,
    watcherEvent,
    watcherEventFields,
    watcherObject,
    watcherPreviewLimit,
  } from '../atoms/watcher-presentation';
  import ContentActions from './ContentActions.svelte';
  import Disclosure from './Disclosure.svelte';
  import RichText from './content/RichText.svelte';
  import CodeBlock from './content/CodeBlock.svelte';
  import StructuredData from './content/StructuredData.svelte';
  import ToolInput from './ToolInput.svelte';
  import { copyToClipboard } from './clipboard';

  let {
    input,
    truncated = false,
    showEvent = true,
  }: {
    input: string;
    truncated?: boolean;
    showEvent?: boolean;
  } = $props();
  let original = $state(false);
  const preview = $derived(parseWatcherInput(input, truncated));
  async function copyOriginal() {
    try {
      await copyToClipboard(input);
    } catch (failure) {
      toast.error(failure instanceof Error ? failure.message : 'Cannot copy review input.');
    }
  }
  function downloadOriginal() {
    const url = URL.createObjectURL(new Blob([input], { type: 'text/plain;charset=utf-8' }));
    const link = document.createElement('a');
    link.href = url;
    link.download = 'watcher-input-preview.txt';
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
</script>

{#snippet referenceMessage(message: Record<string, unknown>)}
  <article
    class="min-w-0 space-y-2 border-l border-border pl-2"
    aria-label={`${String(message.Role)} reference message`}
  >
    <div class="flex flex-wrap gap-2 text-[10px] font-medium text-muted-foreground">
      <span>{dataLabel(String(message.Role))}</span>
      {#if message.InProgress === true}<span>In progress</span>{/if}
    </div>
    {#each Array.isArray(message.Content) ? message.Content.slice(0, 32) : [] as item}
      {@const content = watcherObject(item)}
      {#if content?.Type === 'text' && typeof content.Text === 'string'}
        {#if typeof content.Filename === 'string' && content.Filename}
          <CodeBlock
            text={content.Text}
            filename={content.Filename}
            language={fileLanguage(content.Filename)}
            embedded
            compact
            wrap
          />
        {:else if message.Role === 'tool' && structuredJSON(content.Text)}
          <StructuredData
            value={structuredJSON(content.Text)!.value}
            label="Referenced tool result"
            controls={false}
            compact
          />
        {:else}<RichText text={content.Text} embedded compact />{/if}
      {:else if content}
        <p class="flex min-w-0 items-start gap-1.5 text-muted-foreground">
          <FileText class="mt-0.5 size-3.5 shrink-0" /><span class="min-w-0 break-words"
            >{String(content.Filename || content.MIME || content.Type || 'Attachment')}</span
          >
        </p>
      {/if}
    {/each}
    {#each Array.isArray(message.ToolCalls) ? message.ToolCalls.slice(0, 32) : [] as value}
      {@const call = watcherObject(value)}
      {#if call && typeof call.Name === 'string'}
        <section class="min-w-0" aria-label={`Referenced tool ${call.Name}`}>
          <p class="mb-1 break-words font-mono text-[11px] text-muted-foreground">{call.Name}</p>
          <ToolInput
            call={{
              ID: typeof call.ID === 'string' ? call.ID : '',
              Name: call.Name,
              Input: call.Input,
              InputError: typeof call.InputError === 'string' ? call.InputError : undefined,
            }}
          />
        </section>
      {/if}
    {/each}
    {#if typeof message.Reasoning === 'string' && message.Reasoning}
      <Disclosure title="Supplied reasoning"
        ><RichText text={message.Reasoning} embedded compact /></Disclosure
      >
    {/if}
  </article>
{/snippet}

<section aria-label="Watcher input preview" class="min-w-0 text-xs">
  <div class="flex min-w-0 items-start justify-between gap-2">
    <p class="min-w-0 py-1 text-[10px] text-muted-foreground">
      Supplied to the reviewer{truncated || preview.limited ? ' · Excerpt' : ''}
    </p>
    <ContentActions
      label="Review input actions"
      actions={[
        {
          label: original ? 'Show formatted input' : 'Show original preview',
          select: () => (original = !original),
        },
        { label: 'Copy original preview', select: copyOriginal },
        { label: 'Download original preview', select: downloadOriginal },
      ]}
    />
  </div>
  <div class="max-h-96 min-w-0 space-y-4 overflow-auto overscroll-contain">
    {#if original}
      <pre
        class="whitespace-pre-wrap break-words font-mono leading-5"
        aria-label="Original watcher input preview">{input.slice(0, watcherPreviewLimit)}</pre>
    {:else}
      {#if showEvent && preview.fields.length}
        <StructuredData
          value={Object.fromEntries(preview.fields)}
          label="Review event"
          controls={false}
          compact
        />
      {/if}
      {#each preview.parts as part, index (index)}
        {@const conversation = part.kind.startsWith('conversation_to_')
          ? watcherConversation(part)
          : null}
        {@const call = part.kind === 'tool_call' ? watcherObject(part.value) : null}
        {@const event = part.kind === 'trigger' && part.json ? watcherEvent(part.value) : null}
        <section aria-label={part.label} class="min-w-0 space-y-2">
          {#if part.kind !== 'text'}<h4 class="text-[11px] font-medium text-muted-foreground">
              {part.label}{part.truncated ? ' · Excerpt' : ''}
            </h4>{/if}
          {#if conversation}
            <div class="min-w-0 space-y-3">
              {#each conversation.messages as message}{@render referenceMessage(message)}{/each}
            </div>
            {#if conversation.partial}<p class="text-[10px] text-muted-foreground">
                The saved preview ends inside a message. Its remaining source is available in the
                original preview.
              </p>{/if}
            {#if conversation.omitted}<p class="text-[10px] text-muted-foreground">
                Showing the first 40 messages in this preview. The original preview retains the
                remaining data.
              </p>{/if}
            {#if !conversation.messages.length && !conversation.partial}<p
                class="text-muted-foreground"
              >
                No conversation messages supplied.
              </p>{/if}
          {:else if call && typeof call.Name === 'string'}
            <p class="break-words font-mono text-[11px]">{call.Name}</p>
            <ToolInput
              call={{
                ID: typeof call.ID === 'string' ? call.ID : '',
                Name: call.Name,
                Input: call.Input,
                InputError: typeof call.InputError === 'string' ? call.InputError : undefined,
              }}
            />
          {:else if event}
            <StructuredData
              value={Object.fromEntries(watcherEventFields(event))}
              label={part.label}
              controls={false}
              compact
            />
          {:else if part.json}
            <StructuredData
              value={part.value}
              label={`${part.label} data`}
              controls={false}
              compact
            />
          {:else}
            <RichText text={part.text} embedded compact />
          {/if}
        </section>
      {/each}
      {#if !preview.parts.length}<p class="text-muted-foreground">
          No prompt text is present in this preview.
        </p>{/if}
    {/if}
  </div>
  {#if truncated || preview.limited}<p class="mt-2 text-[10px] text-muted-foreground">
      {preview.limited
        ? 'Display limited to 100,000 characters. Copy and download retain the supplied preview.'
        : 'This is an input excerpt; it may omit earlier context or end before the full input.'}
    </p>{/if}
</section>
