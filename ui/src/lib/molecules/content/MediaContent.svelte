<script lang="ts">
  import { FileText, Download } from 'lucide-svelte';
  import type { Content } from '../../atoms/types';
  import { safeContentURL } from '../../atoms/content';
  let { item }: { item: Content } = $props();
  let container = $state<HTMLElement | null>(null);
  let visible = $state(false);
  let source = $state('');
  let failed = $state(false);
  const image = $derived(
    item.Type === 'image' &&
      ['image/png', 'image/jpeg', 'image/webp', 'image/gif', ''].includes(item.MIME),
  );
  const audio = $derived(item.Type === 'audio');
  const video = $derived(item.Type === 'video');
  $effect(() => {
    const element = container;
    if (!element) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) {
          visible = true;
          observer.disconnect();
        }
      },
      { rootMargin: '200px' },
    );
    observer.observe(element);
    return () => observer.disconnect();
  });
  $effect(() => {
    if (!visible) return;
    failed = false;
    let objectURL = '';
    let resolved = '';
    try {
      if (item.Data && item.Data.length <= 16 * 1024 * 1024) {
        const bytes = Uint8Array.from(atob(item.Data), (character) => character.charCodeAt(0));
        const mime = item.MIME || (image ? 'image/png' : 'application/octet-stream');
        objectURL = URL.createObjectURL(new Blob([bytes], { type: mime }));
        resolved = objectURL;
      } else if (item.URL) {
        resolved = safeContentURL(item.URL, window.location.href, true);
      }
      if (!resolved) failed = true;
    } catch {
      failed = true;
    }
    source = resolved;
    return () => {
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  });
</script>

<div
  bind:this={container}
  class="my-2 min-w-0 space-y-1"
  aria-label={item.Filename || `${item.Type} attachment`}
>
  {#if source && !failed && image}<img
      src={source}
      alt={item.Filename || 'Attached image'}
      loading="lazy"
      class="min-h-8 min-w-8 max-h-80 max-w-full rounded-md object-contain"
      onerror={() => (failed = true)}
    />
  {:else if source && !failed && video}<video
      src={source}
      controls
      preload="metadata"
      playsinline
      class="aspect-video max-h-80 w-full max-w-xl rounded-md bg-black object-contain"
      aria-label={item.Filename || 'Attached video'}
      onerror={() => (failed = true)}
      ><track kind="captions" />Your browser cannot play this video.</video
    >
  {:else if source && !failed && audio}<audio
      src={source}
      controls
      preload="metadata"
      class="max-w-full"
      aria-label={item.Filename || 'Attached audio'}
      onerror={() => (failed = true)}
    ></audio>{/if}
  <div class="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
    <FileText class="size-4 shrink-0" /><span class="min-w-0 break-words"
      >{item.Filename || item.Type}{item.MIME ? ` · ${item.MIME}` : ''}</span
    >
    {#if source}{#if item.Data}<a
          href={source}
          download={item.Filename || 'attachment'}
          class="inline-flex min-h-11 shrink-0 items-center gap-1 px-2 underline"
          ><Download class="size-3" />Download</a
        >{:else}<a
          href={source}
          target="_blank"
          rel="noopener noreferrer"
          class="inline-flex min-h-11 shrink-0 items-center px-2 underline">Open</a
        >{/if}{/if}
  </div>
  {#if failed}<p class="text-xs text-muted-foreground">
      {source
        ? 'Preview unavailable. You can download the attachment.'
        : 'Attachment preview unavailable.'}
    </p>{/if}
</div>
