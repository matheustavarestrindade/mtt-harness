<script lang="ts">
  import type { ToolActivity } from '../atoms/transcript';
  import { bashOutputKind } from '../atoms/bash-output';
  import { plainFileReadPath } from '../atoms/file-actions';
  import { fileLanguage } from '../atoms/content';
  import { structuredJSON } from '../atoms/structured-data';
  import StructuredData from './content/StructuredData.svelte';
  import MessageContent from './content/MessageContent.svelte';
  import TerminalOutput from './content/TerminalOutput.svelte';
  import Disclosure from './Disclosure.svelte';
  let { activity }: { activity: ToolActivity } = $props();
  const path = $derived(plainFileReadPath(activity.call));
  const language = $derived(path ? fileLanguage(path) : 'text');
  const terminal = $derived(['bash', 'process_output'].includes(activity.call.Name));
  const outputKind = $derived(bashOutputKind(activity));
  const content = $derived(
    activity.processNotice && ['missing', 'started'].includes(outputKind)
      ? [
          {
            Type: 'text' as const,
            Text: activity.processNotice.output,
            Data: null,
            MIME: '',
            URL: '',
            Filename: '',
            AudioID: '',
          },
        ]
      : (activity.result?.Content ?? []),
  );
</script>

{#if terminal}
  <TerminalOutput
    text={content
      .filter((part) => part.Type === 'text')
      .map((part) => part.Text)
      .join('\n')}
  />
  <MessageContent content={content.filter((part) => part.Type !== 'text')} embedded />
  {#if outputKind === 'inactivity' && activity.processNotice}
    <Disclosure
      class="mt-1 text-xs text-muted-foreground"
      title={activity.processNotice.kind === 'exit' ? 'Completion output' : 'Background output'}
    >
      <TerminalOutput text={activity.processNotice.output} label="Background process output" />
    </Disclosure>
  {/if}
{:else}
  {#each content as part}
    {@const data = part.Type === 'text' ? structuredJSON(part.Text) : null}
    {#if data}<StructuredData value={data.value} label="Tool result" ansiOutput compact />
    {:else if part.Type === 'text' && part.Text.includes('\x1b')}<TerminalOutput text={part.Text} />
    {:else if part.Type === 'text' && language === 'text' && part.Text.length < 180 && !part.Text.includes('\n')}
      <span class="whitespace-pre-wrap break-words text-xs leading-6">{part.Text}</span>
    {:else}<MessageContent
        content={[part]}
        mode={language === 'markdown' ? 'markdown' : 'code'}
        {language}
        embedded
        compact
      />{/if}
  {/each}
{/if}
