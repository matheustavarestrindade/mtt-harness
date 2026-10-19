<script lang="ts">
  import { Bot, ChevronRight, Link2 } from 'lucide-svelte';
  import type { PluginActivityEntry } from '../atoms/plugin-activity';
  import RichText from './content/RichText.svelte';
  import ContentActions from './ContentActions.svelte';
  import { copyToClipboard } from './clipboard';
  import { toast } from 'svelte-sonner';
  let { entry, earlier = false }: { entry: PluginActivityEntry; earlier?: boolean } = $props();
  let open = $state(false);
  const identifier = $props.id();
  async function copyContext() {
    try {
      await copyToClipboard(
        entry.Notes.map((note) => `${note.Text}\n${(note.References ?? []).join('\n')}`).join(
          '\n\n',
        ),
      );
    } catch (failure) {
      toast.error(failure instanceof Error ? failure.message : 'Cannot copy Sidekick context.');
    }
  }
</script>

<section
  class="assistant-section min-w-0"
  data-sidekick-id={entry.ID}
  aria-label="Sidekick context"
>
  <button
    type="button"
    class="activity-toggle w-full"
    aria-expanded={open}
    aria-controls={identifier}
    onclick={() => (open = !open)}
  >
    <ChevronRight class={`size-3 shrink-0 transition-transform ${open ? 'rotate-90' : ''}`} />
    <Bot class="size-3.5 shrink-0" />
    <span class="shrink-0 font-medium text-foreground">Sidekick</span>
    <span class="min-w-0 truncate" title={entry.Title}>{entry.Title}</span>
    {#if earlier}<span class="ml-auto shrink-0 text-[10px]">Earlier context</span>{/if}
  </button>
  {#if open}
    <div
      id={identifier}
      class="tool-details mt-1 min-w-0 border border-border bg-card/70 p-2 text-xs"
    >
      <div class="max-h-96 min-w-0 space-y-3 overflow-auto overscroll-contain">
        {#each entry.Notes as note}
          <div class="min-w-0">
            <RichText text={note.Text} embedded compact />
            {#if note.References?.length}<div
                class="mt-1 flex items-start gap-2 text-[10px] text-muted-foreground"
                aria-label="Sources"
              >
                <Link2 class="mt-0.5 size-3 shrink-0" />
                <div class="min-w-0 space-y-0.5">
                  {#each note.References as reference}<div class="break-words font-mono">
                      {reference}
                    </div>{/each}
                </div>
              </div>{/if}
          </div>
        {/each}
      </div>
      <div class="mt-2 flex min-w-0 items-center gap-2 text-[10px] text-muted-foreground">
        <time datetime={entry.CreatedAt} title={entry.CreatedAt}
          >{new Date(entry.CreatedAt).toLocaleTimeString([], {
            hour: '2-digit',
            minute: '2-digit',
          })}</time
        >
        {#if entry.InputTokens !== undefined}<span
            title="Added request input; already included in chat token accounting"
            >{entry.Estimated ? '≈ ' : ''}{entry.InputTokens.toLocaleString()} input tokens</span
          >{/if}
        <div class="ml-auto">
          <ContentActions
            label="Sidekick actions"
            actions={[{ label: 'Copy Sidekick context', select: copyContext }]}
          />
        </div>
      </div>
    </div>
  {/if}
</section>
