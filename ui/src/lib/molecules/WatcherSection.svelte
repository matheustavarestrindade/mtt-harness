<script lang="ts">
  import { Eye, ChevronRight } from 'lucide-svelte';
  import type { PluginActivityEntry } from '../atoms/plugin-activity';
  import ActivityIndicator from '../atoms/ActivityIndicator.svelte';
  import * as Collapsible from '../atoms/ui/collapsible';
  import Disclosure from './Disclosure.svelte';
  let { entry }: { entry: PluginActivityEntry } = $props();
  let open = $state(false);
</script>

<Collapsible.Root bind:open class="section-content" data-watcher-run={entry.ID}>
  <Collapsible.Trigger
    class="flex min-h-8 w-full items-center gap-2 py-1 text-left text-xs text-muted-foreground pointer-coarse:min-h-11"
  >
    <ChevronRight class={`size-3 shrink-0 transition-transform ${open ? 'rotate-90' : ''}`} /><Eye
      class="size-3.5 shrink-0"
    /><span class="min-w-0 break-words">{entry.Title}</span><span
      class="ml-auto shrink-0 text-[10px]">{entry.Outcome ?? 'review'}</span
    >{#if entry.Outcome === 'running'}<ActivityIndicator /><span class="sr-only"
        >Watcher reviewing</span
      >{/if}
  </Collapsible.Trigger>
  <Collapsible.Content class="min-w-0 space-y-3 pb-3 pl-5 text-xs">
    {#if open}
      <p class="text-muted-foreground">
        {entry.Trigger?.replaceAll('_', ' ')}{entry.Decision
          ? ` · ${entry.Decision}`
          : ''}{entry.DurationMS ? ` · ${entry.DurationMS} ms` : ''}
      </p>
      {#each entry.Notes ?? [] as note}<p class="whitespace-pre-wrap break-words leading-5">
          {note.Text}
        </p>{/each}
      {#if entry.Error}<p class="break-words text-destructive">{entry.Error}</p>{/if}
      {#if entry.InputPreview}<Disclosure
          title={`Review input${entry.InputTruncated ? ' · excerpt' : ''}`}
        >
          <pre
            class="max-h-72 overflow-auto whitespace-pre-wrap break-words">{entry.InputPreview}</pre>
        </Disclosure>{/if}
    {/if}
  </Collapsible.Content>
</Collapsible.Root>
