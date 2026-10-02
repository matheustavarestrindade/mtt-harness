<script lang="ts">
  import { FileText } from 'lucide-svelte';
  import type { Content } from '../../atoms/types';
  import { contentFileText, fileLanguage, safeContentURL } from '../../atoms/content';
  import RichText from './RichText.svelte';
  import CodeBlock from './CodeBlock.svelte';
  let {
    content,
    mode = 'markdown',
    language = 'text',
    embedded = false,
  }: {
    content: Content[] | null;
    mode?: 'markdown' | 'plain' | 'code';
    language?: string;
    embedded?: boolean;
  } = $props();
  const parts = $derived.by(() => {
    const result: Content[] = [];
    for (const item of content ?? []) {
      const previous = result[result.length - 1];
      if (item.Type === 'text' && previous?.Type === 'text') previous.Text += '\n' + item.Text;
      else result.push({ ...item });
    }
    return result;
  });
</script>

{#each parts as item, index (index)}
  {#if item.Type === 'text' && item.Text}
    {#if mode === 'markdown'}<RichText text={item.Text} {embedded} />
    {:else if mode === 'code'}<CodeBlock text={item.Text} {language} {embedded} />
    {:else}<p class="message-copy">{item.Text}</p>{/if}
  {:else if item.Type === 'file' && (item.MIME?.startsWith('text/') || item.MIME === 'application/json') && contentFileText(item) !== null}
    <CodeBlock
      text={contentFileText(item)!}
      language={item.MIME === 'text/csv'
        ? 'csv'
        : item.MIME === 'application/json'
          ? 'json'
          : fileLanguage(item.Filename)}
      filename={item.Filename}
      {embedded}
    />
  {:else if item.Type !== 'text'}
    {@const url = safeContentURL(item.URL, window.location.href)}
    <div
      class="my-3 flex min-w-0 items-center gap-2 text-xs text-muted-foreground {embedded
        ? 'border-b border-border py-3'
        : 'rounded-lg border border-border p-3'}"
    >
      <FileText class="size-4 shrink-0" /><span class="min-w-0 flex-1 break-words"
        >{item.Filename || item.Type} {item.MIME ? `· ${item.MIME}` : ''}</span
      >{#if url}<a
          href={url}
          target="_blank"
          rel="noopener noreferrer"
          class="inline-flex min-h-11 shrink-0 items-center px-2 underline">Open</a
        >{/if}
    </div>
  {/if}
{/each}
