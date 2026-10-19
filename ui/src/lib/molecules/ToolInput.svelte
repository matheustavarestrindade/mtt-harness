<script lang="ts">
  import type { ToolCall } from '../atoms/types';
  import { fileLanguage } from '../atoms/content';
  import { dataLabel } from '../atoms/structured-data';
  import StructuredData from './content/StructuredData.svelte';
  import CodeBlock from './content/CodeBlock.svelte';
  let { call }: { call: ToolCall } = $props();
  const input = $derived(
    call.Input && typeof call.Input === 'object' && !Array.isArray(call.Input)
      ? (call.Input as Record<string, unknown>)
      : null,
  );
  const command = $derived(
    call.Name === 'bash' && typeof input?.command === 'string' ? input.command : null,
  );
  const actions = $derived(
    call.Name === 'file_actions' && Array.isArray(input?.actions)
      ? (input.actions as Record<string, unknown>[])
      : null,
  );
  const metadata = $derived(
    Object.fromEntries(
      Object.entries(input ?? {})
        .filter(([key]) =>
          command !== null ? key !== 'command' : actions ? key !== 'actions' : true,
        )
        .map(([key, value]) => [
          key,
          command !== null &&
          ['timeout', 'idle_timeout', 'interval'].includes(key) &&
          typeof value === 'number'
            ? `${value} ms`
            : value,
        ]),
    ),
  );
  const language = $derived(typeof input?.path === 'string' ? fileLanguage(input.path) : 'text');
</script>

{#if command !== null}
  <CodeBlock text={command} language="bash" embedded />
  {#if Object.keys(metadata).length}<StructuredData
      value={metadata}
      label="Options"
      controls={false}
    />{/if}
{:else if actions}
  <StructuredData value={metadata} label="Files and return" controls={false} />
  <ol class="mt-2 min-w-0 space-y-3" aria-label="File actions">
    {#each actions.slice(0, 32) as action, index}<li class="min-w-0 border-t border-border/60 pt-2">
        {#if action && typeof action === 'object' && !Array.isArray(action)}
          <h4 class="mb-2 text-xs font-medium">
            {index + 1}. {dataLabel(typeof action.op === 'string' ? action.op : 'Action')}
          </h4>
          {@const options = Object.fromEntries(
            Object.entries(action).filter(
              ([key]) => !['op', 'content', 'old_text', 'new_text'].includes(key),
            ),
          )}
          {#if Object.keys(options).length}<StructuredData
              value={options}
              label={`Action ${index + 1} options`}
              controls={false}
            />{/if}
          {#each ['old_text', 'new_text', 'content'] as field}{#if typeof action[field] === 'string'}
              <p class="mt-2 text-[11px] font-medium text-muted-foreground">
                {field === 'old_text' ? 'Find' : field === 'new_text' ? 'Replace with' : 'Content'}
              </p>
              {#if (action[field] as string).length < 160 && !(action[field] as string).includes('\n')}
                <pre class="whitespace-pre-wrap break-words py-1 text-xs"><code
                    >{action[field]}</code
                  ></pre>
              {:else}<CodeBlock text={action[field] as string} {language} embedded />{/if}
            {/if}{/each}
        {:else}<StructuredData value={action} label={`Action ${index + 1}`} />{/if}
      </li>{/each}
  </ol>
{:else}<StructuredData value={call.Input} label="Tool arguments" />{/if}
