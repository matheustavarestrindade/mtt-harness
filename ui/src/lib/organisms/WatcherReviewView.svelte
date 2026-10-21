<script lang="ts">
  import type { HarnessApi } from '../molecules/api/client';
  import type { WatcherReview } from '../atoms/watcher-presentation';
  import Disclosure from '../molecules/Disclosure.svelte';
  import WatcherReviewDetails from '../molecules/WatcherReviewDetails.svelte';
  import WatcherLiveOutput from './WatcherLiveOutput.svelte';
  let { api, review }: { api: HarnessApi | null; review: WatcherReview } = $props();
  let liveOpen = $state(false),
    activated = false;
  $effect(() => {
    if (!activated && review.status === 'running') {
      activated = true;
      liveOpen = true;
    }
  });
</script>

<div class="min-w-0 space-y-2">
  <WatcherReviewDetails {review} />
  {#if api && review.workerSessionID}<Disclosure
      bind:open={liveOpen}
      title="Reviewer activity"
      class="border-t border-border pt-1"
    >
      {#if liveOpen}{#key review.workerSessionID}<WatcherLiveOutput
            {api}
            sessionID={review.workerSessionID}
          />{/key}{/if}
    </Disclosure>{/if}
</div>
