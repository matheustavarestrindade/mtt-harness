<script lang="ts">
  import type { Snippet } from 'svelte';
  import { ArrowLeftFromLine, Clock3 } from 'lucide-svelte';
  import {
    parseWatcherInput,
    watcherEventFields,
    watcherTriggerLabel,
    watcherStatusLabel,
    type WatcherReview,
  } from '../atoms/watcher-presentation';
  import { durationLabel } from '../atoms/memory';
  import RichText from './content/RichText.svelte';
  import Disclosure from './Disclosure.svelte';
  import WatcherReviewInput from './WatcherReviewInput.svelte';

  let { review, inputContent }: { review: WatcherReview; inputContent?: Snippet } = $props();
  const input = $derived(parseWatcherInput(review.input, review.inputTruncated));
  const fields = $derived.by(() => {
    const trigger = review.trigger || String(input.event?.type ?? input.event?.kind ?? '');
    const values: [string, string][] = [
      ['Trigger', watcherTriggerLabel(trigger) || 'Not recorded'],
    ];
    if (review.decision)
      values.push([
        'Decision',
        review.decision === 'continue'
          ? 'Continue'
          : review.decision === 'interfere'
            ? 'Interfere'
            : review.decision,
      ]);
    if (review.status) values.push(['Outcome', watcherStatusLabel(review.status)]);
    if (review.continueChat !== undefined)
      values.push([
        'Automatic continuation',
        review.continueChat ? 'Allowed' : 'Off — user input requested',
      ]);
    if (input.event)
      values.push(...watcherEventFields(input.event).filter(([key]) => key !== 'Trigger'));
    if (review.model) values.push(['Reviewer model', review.model]);
    return values;
  });
  const timestamp = $derived(
    Number.isFinite(Date.parse(review.createdAt))
      ? new Date(review.createdAt).toLocaleString()
      : '',
  );
</script>

<div class="min-w-0 space-y-2 text-xs" data-watcher-details={review.id}>
  <dl aria-label="Review details" class="grid min-w-0 grid-cols-2 gap-x-4 gap-y-2">
    {#each fields as [label, value]}<div class="min-w-0">
        <dt class="text-[10px] text-muted-foreground">{label}</dt>
        <dd class="break-words leading-5">{value}</dd>
      </div>{/each}
  </dl>
  {#if review.feedback.length}
    <section
      aria-label="Watcher feedback"
      class="grid min-w-0 grid-cols-[.875rem_minmax(0,1fr)] gap-2 border-t border-border pt-2"
    >
      <ArrowLeftFromLine aria-hidden="true" class="mt-1.5 size-3.5 text-muted-foreground" />
      <div class="max-h-80 min-w-0 space-y-2 overflow-auto overscroll-contain">
        {#each review.feedback as text}<RichText {text} embedded compact />{/each}
      </div>
    </section>
  {/if}
  {#if review.error}<p
      role="status"
      class="max-h-52 overflow-auto overscroll-contain whitespace-pre-wrap break-words text-destructive"
    >
      {review.error}
    </p>{/if}
  {#if review.input}<div class="min-w-0 border-t border-border pt-1">
      <Disclosure title="Review input">
        {#if inputContent}{@render inputContent()}{:else}<WatcherReviewInput
            input={review.input}
            truncated={review.inputTruncated}
            showEvent={!input.event}
          />{/if}
      </Disclosure>
    </div>{/if}
  <div
    class="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-[10px] text-muted-foreground"
  >
    {#if timestamp}<time datetime={review.createdAt}>{timestamp}</time>{/if}
    {#if review.durationMS !== undefined}<span
        class="inline-flex items-center gap-1"
        title={`${review.durationMS} ms`}
        ><Clock3 class="size-3" />{durationLabel(review.durationMS)}</span
      >{/if}
    {#if review.inputTokens !== undefined}<span
        title="Initial reviewer request size, including instructions and tools. This is not the conversation-window size or the visible preview length."
        >Initial request: {review.estimated ? '≈ ' : ''}{review.inputTokens.toLocaleString()} tokens</span
      >{/if}
  </div>
</div>
