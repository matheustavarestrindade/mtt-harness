<script lang="ts">
  import DOMPurify from 'dompurify';
  let { text }: { text: string } = $props();
  let html = $state<string | null>(null);
  let unavailable = $state(false);
  $effect(() => {
    const source = text.slice(0, 32_000);
    let cancelled = false;
    html = null;
    unavailable = false;
    void import('ansi_up')
      .then(({ AnsiUp }) => {
        const formatter = new AnsiUp();
        formatter.escape_html = true;
        formatter.url_allowlist = {};
        const rendered = DOMPurify.sanitize(formatter.ansi_to_html(source), {
          ALLOWED_TAGS: ['span'],
          ALLOWED_ATTR: ['style'],
        });
        if (!cancelled) html = rendered;
      })
      .catch(() => {
        if (!cancelled) unavailable = true;
      });
    return () => {
      cancelled = true;
    };
  });
</script>

<div data-ansi-text class="whitespace-pre-wrap break-words font-mono text-xs leading-5">
  {#if html !== null}{@html html}{:else}{text.slice(0, 32_000)}{/if}
</div>
{#if unavailable}<p class="text-[10px] text-muted-foreground">Color preview unavailable.</p>{/if}
