<script lang="ts">
  import {
    ArrowDown,
    FileText,
    GitCompareArrows,
    Minus,
    Plus,
    Clock3,
    SlidersHorizontal,
    Play,
    BellOff,
    FileSearch,
    Pencil,
    Trash2,
    ListPlus,
    Search,
  } from 'lucide-svelte';
  import { Button } from '../atoms/ui/button';
  import type { ToolCall } from '../atoms/types';
  import { fileLanguage } from '../atoms/content';
  import { dataLabel } from '../atoms/structured-data';
  import { fileActionRange, toolDuration, toolObject } from '../atoms/tool-presentation';
  import StructuredData from './content/StructuredData.svelte';
  import CodeBlock from './content/CodeBlock.svelte';
  let { call }: { call: ToolCall } = $props();
  let showOptions = $state(false);
  const input = $derived(toolObject(call.Input));
  const command = $derived(
    call.Name === 'bash' && typeof input?.command === 'string' ? input.command : null,
  );
  const actions = $derived(
    call.Name === 'file_actions' && Array.isArray(input?.actions)
      ? input.actions.slice(0, 32)
      : null,
  );
  const paths = $derived(
    typeof input?.path === 'string'
      ? [input.path]
      : Array.isArray(input?.paths)
        ? input.paths.filter((path): path is string => typeof path === 'string').slice(0, 32)
        : [],
  );
  const language = $derived(paths.length === 1 ? fileLanguage(paths[0]) : 'text');
  const options = $derived(
    Object.fromEntries(
      Object.entries(input ?? {}).filter(
        ([key]) => !['command', 'path', 'paths', 'actions'].includes(key),
      ),
    ),
  );
  const extraBashOptions = $derived(
    Object.fromEntries(
      Object.entries(options).filter(
        ([key]) => !['timeout', 'idle_timeout', 'wait', 'notify'].includes(key),
      ),
    ),
  );
  const actionIcons = {
    replace: GitCompareArrows,
    read: FileSearch,
    write: Pencil,
    delete: Trash2,
    append: ListPlus,
    prepend: ListPlus,
    list: FileSearch,
    glob: Search,
  };
</script>

{#if command !== null}
  <CodeBlock text={command} language="bash" embedded compact />
  {#if Object.keys(options).length}<div
      class="flex flex-wrap items-center gap-x-3 gap-y-1 text-[10px] text-muted-foreground"
    >
      {#if typeof input?.timeout === 'number'}<span
          class="inline-flex items-center gap-1"
          title={`Timeout: ${input.timeout} ms`}
          aria-label={`Timeout ${toolDuration(input.timeout)}`}
          ><Clock3 class="size-3" />{toolDuration(input.timeout)}</span
        >{/if}
      {#if typeof input?.idle_timeout === 'number'}<span
          title={`Inactivity: ${input.idle_timeout} ms`}
          >Idle {input.idle_timeout === 0 ? 'off' : toolDuration(input.idle_timeout)}</span
        >{/if}
      {#if input?.wait === false}<span class="inline-flex items-center gap-1" title="wait: false"
          ><Play class="size-3" />Background</span
        >{/if}
      {#if input?.notify === 'none'}<BellOff
          class="size-3"
          aria-label="Notifications off"
        />{:else if input?.notify && input.notify !== 'exit'}<span
          >Notify {String(input.notify)}</span
        >{/if}
      {#if Object.keys(extraBashOptions).length}<StructuredData
          value={extraBashOptions}
          label="Command options"
          controls={false}
          compact
        />{/if}
    </div>{/if}
{:else if actions}
  <div class="flex min-w-0 items-start gap-2">
    <div class="min-w-0 flex-1 py-1">
      {#each paths as path}<div class="flex min-w-0 items-start gap-1.5">
          <FileText class="mt-0.5 size-3 shrink-0 text-muted-foreground" /><span
            class="break-all font-mono text-xs">{path}</span
          >
        </div>{/each}
    </div>
    {#if Object.keys(options).length}<Button
        variant="ghost"
        size="icon"
        class="size-7 shrink-0 text-muted-foreground pointer-coarse:size-11"
        aria-label="Tool options"
        aria-expanded={showOptions}
        title="Tool options"
        onclick={() => (showOptions = !showOptions)}><SlidersHorizontal class="size-3.5" /></Button
      >{/if}
  </div>
  {#if showOptions}<div class="pb-2">
      <StructuredData value={options} label="Tool options" controls={false} compact />
    </div>{/if}
  <ol class="min-w-0 space-y-2" aria-label="File actions">
    {#each actions as value, index}
      {@const action = toolObject(value)}
      {#if action}
        {@const operation = typeof action.op === 'string' ? action.op : 'Action'}
        {@const Icon = Object.hasOwn(actionIcons, operation)
          ? actionIcons[operation as keyof typeof actionIcons]
          : Pencil}
        {@const range = fileActionRange(action)}
        {@const metadata = Object.fromEntries(
          Object.entries(action).filter(
            ([key]) =>
              !['op', 'content', 'old_text', 'new_text', 'start_line', 'end_line'].includes(key),
          ),
        )}
        <li class="min-w-0" aria-label={`${dataLabel(operation)} ${index + 1}`}>
          {#if operation !== 'read' || actions.length > 1 || range || Object.keys(metadata).length}
            <div
              class="flex min-w-0 flex-wrap items-center gap-1.5 py-1 text-[10px] text-muted-foreground"
            >
              <span title={dataLabel(operation)}><Icon class="size-3.5" /></span>
              {#if actions.length > 1}<span>{index + 1}</span>{/if}
              {#if range}<span class="font-mono">{range}</span>{/if}
              {#if Object.keys(metadata).length}<StructuredData
                  value={metadata}
                  label="Action options"
                  controls={false}
                  compact
                />{/if}
            </div>
          {/if}
          {#if typeof action.old_text === 'string' && typeof action.new_text === 'string'}
            <div
              class="grid min-w-0 grid-cols-[.75rem_minmax(0,1fr)] gap-x-1.5"
              aria-label={`Replacement ${index + 1}`}
            >
              <Minus aria-hidden="true" class="mt-1.5 size-3 text-muted-foreground" /><CodeBlock
                text={action.old_text}
                {language}
                compact
                embedded
                wrap
                label="Original text"
              />
              <ArrowDown aria-hidden="true" class="my-0.5 size-3 text-muted-foreground" /><span
              ></span>
              <Plus aria-hidden="true" class="mt-1.5 size-3 text-muted-foreground" /><CodeBlock
                text={action.new_text}
                {language}
                compact
                embedded
                wrap
                label="Replacement text"
              />
            </div>
          {:else if typeof action.content === 'string'}<CodeBlock
              text={action.content}
              {language}
              compact
              embedded
              wrap
              label="File content"
            />{/if}
        </li>
      {:else}<li><StructuredData {value} label="Action data" compact /></li>{/if}
    {/each}
  </ol>
{:else}<StructuredData value={call.Input} label="Tool arguments" compact />{/if}
