<script lang="ts">
  import type { ToolActivity } from '../atoms/transcript';
  import { plainFileReadPath } from '../atoms/file-actions';
  import { fileLanguage } from '../atoms/content';
  import { structuredJSON } from '../atoms/structured-data';
  import StructuredData from './content/StructuredData.svelte';
  import MessageContent from './content/MessageContent.svelte';
  import TerminalOutput from './content/TerminalOutput.svelte';
  let { activity }: { activity: ToolActivity } = $props();
  const path = $derived(plainFileReadPath(activity.call));
  const language = $derived(path ? fileLanguage(path) : 'text');
  const terminal = $derived(['bash', 'process_output'].includes(activity.call.Name));
  const content = $derived(activity.result?.Content ?? []);
</script>

{#if terminal}
  <TerminalOutput
    text={content
      .filter((part) => part.Type === 'text')
      .map((part) => part.Text)
      .join('\n')}
  />
  <MessageContent content={content.filter((part) => part.Type !== 'text')} embedded />
{:else}
  {#each content as part}
    {@const data = part.Type === 'text' ? structuredJSON(part.Text) : null}
    {#if data}<StructuredData value={data.value} label="Tool result" />
    {:else if part.Type === 'text' && part.Text.includes('\x1b')}<TerminalOutput text={part.Text} />
    {:else}<MessageContent
        content={[part]}
        mode={language === 'markdown' ? 'markdown' : 'code'}
        {language}
        embedded
      />{/if}
  {/each}
{/if}
