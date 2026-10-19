<script lang="ts">
  import { Copy, Download, Check } from 'lucide-svelte';
  import { toast } from 'svelte-sonner';
  import { Button } from '../../atoms/ui/button';
  import { copyToClipboard } from '../clipboard';
  import type CsvViewer from './CsvViewer.svelte';
  import ContentActions from '../ContentActions.svelte';
  let {
    text,
    language = 'text',
    filename = '',
    embedded = false,
    compact = false,
    wrap = false,
    label = '',
  }: {
    text: string;
    language?: string;
    filename?: string;
    embedded?: boolean;
    compact?: boolean;
    wrap?: boolean;
    label?: string;
  } = $props();
  let source = $state(false);
  let expanded = $state(false);
  let highlighted = $state('');
  let copied = $state(false);
  let CsvPreview = $state<typeof CsvViewer | null>(null);
  let csvError = $state('');
  const normalizedLanguage = $derived(language.toLowerCase());
  const csv = $derived(['csv', 'tsv'].includes(normalizedLanguage));
  const preview = $derived(expanded ? text : text.slice(0, 60_000));
  $effect(() => {
    if (!csv || source || CsvPreview) return;
    let current = true;
    csvError = '';
    void import('./CsvViewer.svelte')
      .then((module) => {
        if (current) CsvPreview = module.default;
      })
      .catch(() => {
        if (current) csvError = 'The table preview is unavailable. Select Source to read the data.';
      });
    return () => {
      current = false;
    };
  });
  $effect(() => {
    const content = preview;
    const grammar = normalizedLanguage;
    highlighted = '';
    let current = true;
    if (!csv && content.length <= 30_000 && !['text', 'plaintext', ''].includes(grammar)) {
      void import('./highlight')
        .then(({ highlightCode }) => {
          if (current) highlighted = highlightCode(content, grammar) ?? '';
        })
        .catch(() => {
          /* Plain text remains usable if the optional chunk cannot load. */
        });
    }
    return () => {
      current = false;
    };
  });
  async function copyCode() {
    try {
      await copyToClipboard(text);
      copied = true;
      toast.success('Copied code');
    } catch (failure) {
      toast.error(failure instanceof Error ? failure.message : 'Could not copy code.');
    }
  }
  function downloadSource() {
    const url = URL.createObjectURL(
      new Blob([text], { type: csv ? 'text/csv;charset=utf-8' : 'text/plain;charset=utf-8' }),
    );
    const link = document.createElement('a');
    link.href = url;
    link.download = filename || (csv ? `data.${normalizedLanguage}` : 'source.txt');
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
</script>

<section
  aria-label={label || `${csv ? 'CSV' : 'Code'} block`}
  class="content-block min-w-0 {compact
    ? 'compact-code'
    : embedded
      ? 'my-2'
      : 'my-4 overflow-hidden rounded-xl border border-border bg-card/40'}"
>
  {#if compact}
    <div class="flex min-w-0 items-start gap-1">
      <div class="min-w-0 flex-1">
        {#if filename}<span class="block break-all text-[11px] text-muted-foreground"
            >{filename}</span
          >{/if}
        {#if csv && !source}
          {#if CsvPreview}<CsvPreview
              {text}
              delimiter={normalizedLanguage === 'tsv' ? '\t' : ','}
            />{:else}<p role="status" class="text-xs">{csvError || 'Loading table…'}</p>{/if}
        {:else}<pre
            class="code-content max-h-[32rem] overflow-auto py-1"
            class:code-wrap={wrap}><code
              >{#if highlighted}{@html highlighted}{:else}{preview}{/if}</code
            ></pre>{/if}
      </div>
      <ContentActions
        label={label ? `${label} actions` : 'Code actions'}
        actions={[
          ...(csv
            ? [
                {
                  label: source ? 'Show table' : 'Show source',
                  select: () => {
                    source = !source;
                  },
                },
              ]
            : []),
          { label: 'Copy code', select: copyCode },
          { label: 'Download source', select: downloadSource },
        ]}
      />
    </div>
  {:else}
    <div
      class="flex min-h-11 flex-wrap items-center gap-x-2 text-xs text-muted-foreground {embedded
        ? ''
        : 'border-b border-border px-3'}"
    >
      <span class="min-w-0 flex-1 truncate font-mono"
        >{filename || normalizedLanguage || 'text'}</span
      >
      {#if csv}<Button variant="ghost" class="h-11 px-2 text-xs" onclick={() => (source = !source)}
          >{source ? 'Table' : 'Source'}</Button
        >{/if}
      <Button
        variant="ghost"
        size="icon"
        class="icon-button"
        aria-label="Download source"
        title="Download source"
        onclick={downloadSource}><Download class="size-3.5" /></Button
      >
      <Button
        variant="ghost"
        size="icon"
        class="icon-button"
        aria-label="Copy code"
        title="Copy code"
        onclick={() => void copyCode()}
        >{#if copied}<Check class="size-3.5" />{:else}<Copy class="size-3.5" />{/if}</Button
      >
    </div>
    {#if csv && !source}
      {#if CsvPreview}<CsvPreview {text} delimiter={normalizedLanguage === 'tsv' ? '\t' : ','} />
      {:else}<p role="status" class="p-4 text-xs text-muted-foreground">
          {csvError || 'Loading table…'}
        </p>{/if}
    {:else}
      <pre class="code-content max-h-[32rem] overflow-auto {embedded ? 'py-2' : 'p-4'}"><code
          >{#if highlighted}{@html highlighted}{:else}{preview}{/if}</code
        ></pre>
    {/if}
  {/if}
  {#if !expanded && text.length > preview.length}<div
      class="border-t border-border py-2 {embedded ? '' : 'px-3'}"
    >
      <p class="text-xs text-muted-foreground">
        Preview limited to 60,000 characters. Copy and download include the full source.
      </p>
      <Button variant="ghost" class="mt-1 h-11 text-xs" onclick={() => (expanded = true)}
        >Show full source</Button
      >
    </div>{/if}
</section>

<style>
  .code-wrap {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
</style>
