<script lang="ts">
  import { toast } from 'svelte-sonner';
  import { Button } from '../../atoms/ui/button';
  import {
    dataLabel,
    structuredData,
    tableColumns,
    type DataNode,
  } from '../../atoms/structured-data';
  import { copyToClipboard } from '../clipboard';
  import AnsiText from './AnsiText.svelte';
  import ContentActions from '../ContentActions.svelte';
  let {
    value,
    label = 'Data',
    controls = true,
    ansiOutput = false,
    compact = false,
  }: {
    value: unknown;
    label?: string;
    controls?: boolean;
    ansiOutput?: boolean;
    compact?: boolean;
  } = $props();
  let entryLimit = $state(30);
  const root = $derived(structuredData(value, entryLimit));
  function source() {
    return JSON.stringify(value, null, 2) ?? 'null';
  }
  async function copy() {
    try {
      await copyToClipboard(source());
      toast.success(`Copied ${label.toLowerCase()}`);
    } catch {
      toast.error('Could not copy data.');
    }
  }
  function download() {
    const url = URL.createObjectURL(new Blob([source()], { type: 'application/json' }));
    const link = document.createElement('a');
    link.href = url;
    link.download = `${label.toLowerCase().replace(/\s+/g, '-')}.json`;
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
</script>

{#snippet nodeView(node: DataNode)}
  {@const columns = tableColumns(node)}
  {#if columns.length}
    <div class="min-w-0 overflow-x-auto">
      <table aria-label={label} class="w-full border-collapse text-left text-xs">
        <thead
          ><tr
            >{#each columns as column}<th
                class="border-b border-border px-2 py-2 font-medium text-muted-foreground"
                >{dataLabel(column)}</th
              >{/each}</tr
          ></thead
        >
        <tbody
          >{#each node.children as row}<tr
              >{#each columns as column}{@const field = row.children.find(
                  (field) => field.key === column,
                )}<td
                  class="max-w-72 whitespace-pre-wrap break-words border-b border-border/50 px-2 py-2 align-top"
                  >{#if field}{@render nodeView(field)}{:else}—{/if}</td
                >{/each}</tr
            >{/each}</tbody
        >
      </table>
    </div>
  {:else if node.kind === 'object' && node.children.length}
    <dl class="min-w-0 {compact ? 'space-y-1' : 'space-y-2'}">
      {#each node.children as child}<div
          class="grid min-w-0 gap-x-2 gap-y-1 {compact
            ? 'grid-cols-[auto_minmax(0,1fr)]'
            : 'grid-cols-1 sm:grid-cols-[minmax(5rem,9rem)_minmax(0,1fr)]'}"
        >
          <dt class="break-words text-[11px] font-medium text-muted-foreground" title={child.key}>
            {dataLabel(child.key)}
          </dt>
          <dd class="min-w-0">{@render nodeView(child)}</dd>
        </div>{/each}
    </dl>
  {:else if node.kind === 'array' && node.children.length}
    <ol class="min-w-0 space-y-2">
      {#each node.children as child}<li class="flex min-w-0 gap-2">
          <span class="shrink-0 text-[10px] tabular-nums text-muted-foreground">{child.key}.</span>
          <div class="min-w-0 flex-1">{@render nodeView(child)}</div>
        </li>{/each}
    </ol>
  {:else if ansiOutput && node.kind === 'text' && node.text.includes('\x1b')}
    <AnsiText text={node.text} />
  {:else}
    <span
      class="block whitespace-pre-wrap break-words text-xs leading-5 {node.kind === 'text'
        ? 'font-mono'
        : ''} {node.kind === 'empty' || node.kind === 'omitted' ? 'text-muted-foreground' : ''}"
      >{node.text || '(empty text)'}</span
    >
  {/if}
  {#if node.omitted > 0}<p class="mt-1 text-[10px] text-muted-foreground">
      {node.omitted}
      {node.kind === 'text' ? 'characters' : 'entries'} omitted.
    </p>{/if}
{/snippet}

<section aria-label={label} class="min-w-0">
  <div class={compact ? 'flex min-w-0 items-start gap-1' : ''}>
    <div class="min-w-0 flex-1">{@render nodeView(root)}</div>
    {#if compact && controls}<ContentActions
        label={`${label} actions`}
        actions={[
          { label: `Copy ${label.toLowerCase()}`, select: copy },
          { label: `Download ${label.toLowerCase()}`, select: download },
        ]}
      />{/if}
  </div>
  {#if controls && (!compact || (root.omitted > 0 && entryLimit < 100 && root.kind !== 'text'))}<div
      class="mt-1 flex flex-wrap items-center justify-end gap-1"
    >
      {#if root.omitted > 0 && entryLimit < 100 && root.kind !== 'text'}<Button
          variant="ghost"
          class="h-11 text-xs"
          onclick={() => (entryLimit = Math.min(100, entryLimit + 30))}>Show more entries</Button
        >{/if}
      {#if !compact}<ContentActions
          label={`${label} actions`}
          actions={[
            { label: `Copy ${label.toLowerCase()}`, select: copy },
            { label: `Download ${label.toLowerCase()}`, select: download },
          ]}
        />{/if}
    </div>{/if}
</section>
