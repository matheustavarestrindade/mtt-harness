<script lang="ts">
  import { untrack } from 'svelte';
  import type { HarnessApi } from '../molecules/api/client';
  import Button from '../atoms/ui/button/button.svelte';
  import WatcherReviewInput from '../molecules/WatcherReviewInput.svelte';
  import { loadReviewerInput } from '../molecules/reviewer-input';

  let {
    api,
    sessionID,
    preview,
    truncated,
  }: { api: HarnessApi | null; sessionID?: string; preview: string; truncated: boolean } = $props();
  const scopeAPI = $derived(api),
    scopeSession = $derived(sessionID),
    scopePreview = $derived(preview);
  let full = $state<string | null>(null),
    loading = $state(false),
    error = $state('');
  let generation = 0,
    controller: AbortController | undefined;
  $effect(() => {
    scopeAPI;
    scopeSession;
    scopePreview;
    untrack(() => {
      generation++;
      controller?.abort();
      controller = undefined;
      full = null;
      loading = false;
      error = '';
    });
    return () => {
      generation++;
      controller?.abort();
    };
  });
  async function loadFullPrompt() {
    const client = scopeAPI,
      identifier = scopeSession,
      prefix = scopePreview;
    if (!client || !identifier || loading) return;
    controller?.abort();
    controller = new AbortController();
    const signal = controller.signal,
      current = ++generation;
    loading = true;
    error = '';
    try {
      const text = await loadReviewerInput(client, identifier, signal);
      if (!text.startsWith(prefix))
        throw new Error('The saved input does not match the recorded review preview.');
      if (current === generation && !signal.aborted) full = text;
    } catch (failure) {
      if (current === generation && !signal.aborted)
        error =
          failure instanceof Error ? failure.message : 'Cannot load the saved reviewer input.';
    } finally {
      if (current === generation) loading = false;
    }
  }
  function showPreview() {
    generation++;
    controller?.abort();
    full = null;
    loading = false;
    error = '';
  }
</script>

<div class="min-w-0 space-y-2" aria-label="Reviewer prompt inspector">
  <div class="flex min-w-0 flex-wrap items-center justify-between gap-2">
    <p class="min-w-0 flex-1 text-[10px] text-muted-foreground">
      {#if full !== null}Loaded from the reviewer's saved input message.
      {:else}The saved preview is limited to the first 16 KiB. Conversation text can continue beyond
        it.{/if}
    </p>
    {#if api && sessionID}<Button
        variant="outline"
        size="sm"
        class="shrink-0 pointer-coarse:min-h-11"
        disabled={loading}
        onclick={full === null ? loadFullPrompt : showPreview}
      >
        {loading ? 'Loading prompt…' : full === null ? 'Load full prompt' : 'Show saved preview'}
      </Button>{/if}
  </div>
  {#if error}<p role="alert" class="break-words text-xs text-destructive">{error}</p>{/if}
  <WatcherReviewInput
    input={full ?? preview}
    truncated={full === null && truncated}
    showEvent={false}
    source={full === null ? 'preview' : 'saved'}
  />
</div>
