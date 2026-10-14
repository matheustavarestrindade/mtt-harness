<script lang="ts">
  import type { Content } from '../../atoms/types';
  import { contentFileText, fileLanguage } from '../../atoms/content';
  import { textAttachmentPrefix } from '../../atoms/attachments';
  import RichText from './RichText.svelte';
  import CodeBlock from './CodeBlock.svelte';
  import MediaContent from './MediaContent.svelte';
  let {
    content,
    mode = 'markdown',
    language = 'text',
    embedded = false,
    compact = false,
  }: {
    content: Content[] | null;
    mode?: 'markdown' | 'plain' | 'code';
    language?: string;
    embedded?: boolean;
    compact?: boolean;
  } = $props();
  const parts = $derived.by(() => {
    const result: Content[] = [];
    for (const item of content ?? []) {
      const previous = result[result.length - 1];
      if (item.Type === 'text' && previous?.Type === 'text' && !item.Filename && !previous.Filename)
        previous.Text += '\n' + item.Text;
      else result.push({ ...item });
    }
    return result;
  });
</script>

{#each parts as item, index (index)}
  {#if item.Type === 'text' && item.Filename}
    {@const prefix = textAttachmentPrefix(item.Filename)}
    <CodeBlock
      text={item.Text.startsWith(prefix) ? item.Text.slice(prefix.length) : item.Text}
      language={fileLanguage(item.Filename)}
      filename={item.Filename}
      {embedded}
      {compact}
    />
  {:else if item.Type === 'text' && item.Text}
    {#if mode === 'markdown'}<RichText text={item.Text} {embedded} {compact} />
    {:else if mode === 'code'}<CodeBlock text={item.Text} {language} {embedded} {compact} />
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
      {compact}
    />
  {:else if item.Type !== 'text'}
    <MediaContent {item} />
  {/if}
{/each}
