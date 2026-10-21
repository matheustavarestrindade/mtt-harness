<script lang="ts">
  import { Eye, ChevronRight } from 'lucide-svelte';
  import type { PluginActivityEntry } from '../atoms/plugin-activity';
  import ActivityIndicator from '../atoms/ActivityIndicator.svelte';
  import * as Collapsible from '../atoms/ui/collapsible';
  import {
    watcherReviewFromActivity,
    watcherReviewBecameInactive,
    watcherStatusLabel,
  } from '../atoms/watcher-presentation';
  import WatcherReviewView from './WatcherReviewView.svelte';
  import type { HarnessApi } from '../molecules/api/client';
  let { entry, api }: { entry: PluginActivityEntry; api: HarnessApi | null } = $props();
  let open = $state(false);
  let activated = false,
    questionShown = false;
  let previousOutcome: string | undefined;
  $effect(() => {
    const outcome = entry.Outcome;
    const finished = watcherReviewBecameInactive(previousOutcome, outcome);
    previousOutcome = outcome;
    if (outcome === 'user_input_requested' && !questionShown) {
      questionShown = true;
      open = true;
    } else if (finished && outcome !== 'user_input_requested') {
      open = false;
    } else if (!activated && outcome === 'running') {
      activated = true;
      open = true;
    }
  });
</script>

<Collapsible.Root
  bind:open
  role="group"
  aria-label={`Watcher ${entry.Title}`}
  class="assistant-section min-w-0"
  data-watcher-run={entry.ID}
>
  <Collapsible.Trigger class="activity-toggle w-full">
    <ChevronRight class={`size-3 shrink-0 transition-transform ${open ? 'rotate-90' : ''}`} /><Eye
      class="size-3.5 shrink-0"
    /><span class="min-w-0 break-words">{entry.Title}</span><span
      class="ml-auto shrink-0 text-[10px]">{watcherStatusLabel(entry.Outcome ?? 'review')}</span
    >{#if entry.Outcome === 'running'}<ActivityIndicator /><span class="sr-only"
        >Watcher reviewing</span
      >{/if}
  </Collapsible.Trigger>
  <Collapsible.Content
    class="tool-details mt-1 min-w-0 border border-border bg-card/70 p-2 text-xs"
  >
    {#if open}
      <WatcherReviewView {api} review={watcherReviewFromActivity(entry)} />
    {/if}
  </Collapsible.Content>
</Collapsible.Root>
