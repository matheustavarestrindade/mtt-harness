<script lang="ts">
  import { Button } from '../../atoms/ui/button';
  import { parseMarkdown, markdownPreviewLimit } from './markdown';
  import CodeBlock from './CodeBlock.svelte';
  let { text, embedded = false }: { text: string; embedded?: boolean } = $props();
  let source = $state(false);
  const preview = $derived(text.slice(0, markdownPreviewLimit));
  const blocks = $derived.by(() => {
    try {
      return parseMarkdown(preview);
    } catch {
      return [{ type: 'code' as const, text: preview, language: 'text' }];
    }
  });
</script>

{#if source}<CodeBlock {text} {embedded} />
{:else}
  <div class="rich-text min-w-0" class:embedded-content={embedded}>
    {#each blocks as block, index (index)}
      {#if block.type === 'code'}<CodeBlock
          text={block.text}
          language={block.language}
          {embedded}
        />
      {:else}<div class="markdown min-w-0">{@html block.html}</div>{/if}
    {/each}
  </div>
  {#if text.length > markdownPreviewLimit}<p class="mt-3 text-xs text-muted-foreground">
      Large message preview limited to 100,000 characters.
    </p>
    <Button variant="outline" class="mt-2 h-11 text-xs" onclick={() => (source = true)}
      >View full message source</Button
    >{/if}
{/if}
