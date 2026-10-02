<script lang="ts">
  import Papa from 'papaparse';
  let { text, delimiter = ',' }: { text: string; delimiter?: string } = $props();
  let header = $state(true);
  const preview = $derived(text.slice(0, 1_000_000));
  const result = $derived(
    Papa.parse<string[]>(preview, {
      delimiter,
      skipEmptyLines: 'greedy',
      dynamicTyping: false,
      preview: 202,
    }),
  );
  const width = $derived(Math.min(50, Math.max(0, ...result.data.map((row) => row.length))));
  const columns = $derived(
    Array.from({ length: width }, (_, index) =>
      header ? result.data[0]?.[index] || `Column ${index + 1}` : `Column ${index + 1}`,
    ),
  );
  const rows = $derived((header ? result.data.slice(1) : result.data).slice(0, 200));
  const limited = $derived(
    result.meta.truncated ||
      result.data.length > rows.length + (header ? 1 : 0) ||
      text.length > 1_000_000 ||
      result.data.some((row) => row.length > 50),
  );
</script>

<div class="px-3 py-2 text-xs text-muted-foreground">
  <label class="inline-flex min-h-11 cursor-pointer items-center gap-2"
    ><input type="checkbox" bind:checked={header} class="size-4 accent-primary" />First row is a
    header</label
  >
  <span class="ml-3">{rows.length} rows · {width} columns{limited ? ' · Preview limited' : ''}</span
  >
</div>
{#if result.errors.length}<p role="alert" class="px-4 pb-3 text-xs text-destructive">
    CSV format issue: {result.errors[0].message}. Use Source to inspect the original.
  </p>{/if}
<!-- svelte-ignore a11y_no_noninteractive_tabindex (Keyboard access to horizontally scrollable data.) -->
<div
  role="region"
  aria-label="CSV table"
  tabindex="0"
  class="max-h-[28rem] max-w-full overflow-auto border-t border-border"
>
  <table class="w-full border-collapse text-left text-xs">
    <caption class="sr-only">CSV data preview</caption>
    <thead class="sticky top-0 bg-secondary"
      ><tr
        >{#each columns as column}<th
            scope="col"
            class="min-w-24 border-b border-border px-4 py-3 font-medium">{column}</th
          >{/each}</tr
      ></thead
    >
    <tbody
      >{#each rows as row}<tr class="border-b border-border last:border-0 hover:bg-muted/30"
          >{#each columns as _, index}<td
              class="max-w-80 px-4 py-3 whitespace-pre-wrap break-words [overflow-wrap:anywhere]"
              >{row[index] ?? ''}</td
            >{/each}</tr
        >{/each}</tbody
    >
  </table>
</div>
{#if limited}<p class="px-4 py-3 text-xs leading-5 text-muted-foreground">
    The preview shows up to 200 rows and 50 columns from the first 1,000,000 characters. Source,
    copy, and download retain all data.
  </p>{/if}
