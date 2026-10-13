<script lang="ts">
  import { FileText, Film, Image, LoaderCircle, Music, X } from 'lucide-svelte';
  import { Button } from '../atoms/ui/button';
  import { attachmentIssue, attachmentSize, type DraftAttachment } from '../atoms/attachments';
  import type { Model } from '../atoms/types';
  let {
    items,
    model,
    protocol,
    onRemove,
    disabled = false,
  }: {
    items: DraftAttachment[];
    model?: Model;
    protocol?: string;
    onRemove: (id: string) => void;
    disabled?: boolean;
  } = $props();
</script>

<ul
  aria-label="Attachments"
  class="flex max-h-44 min-w-0 flex-wrap gap-2 overflow-y-auto px-1 pt-1"
>
  {#each items as item (item.id)}
    {@const issue = attachmentIssue(item, model, protocol)}
    <li
      class="flex min-w-0 max-w-full items-center gap-2 rounded-lg border bg-muted/40 pl-2 text-xs"
      class:border-destructive={Boolean(issue) && item.status !== 'reading'}
    >
      {#if item.status === 'reading'}<LoaderCircle class="size-5 shrink-0 animate-spin" />
      {:else if item.kind === 'image' && item.preview}<img
          src={item.preview}
          alt=""
          class="size-10 shrink-0 rounded object-cover"
        />
      {:else if item.kind === 'image'}<Image class="size-5 shrink-0" />
      {:else if item.kind === 'video'}<Film class="size-5 shrink-0" />
      {:else if item.kind === 'audio'}<Music class="size-5 shrink-0" />
      {:else}<FileText class="size-5 shrink-0" />{/if}
      <div class="min-w-0 py-1">
        <p class="max-w-48 truncate font-medium" title={item.name}>{item.name}</p>
        <p class="text-muted-foreground">
          {item.kind === 'text' ? 'Text' : item.kind} · {attachmentSize(item.size)}
        </p>
        {#if issue}<p
            class="max-w-64 break-words"
            class:text-destructive={item.status !== 'reading'}
          >
            {issue}
          </p>{/if}
      </div>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        class="size-11 shrink-0"
        aria-label={`Remove ${item.name}`}
        {disabled}
        onclick={() => onRemove(item.id)}><X class="size-4" /></Button
      >
    </li>
  {/each}
</ul>
