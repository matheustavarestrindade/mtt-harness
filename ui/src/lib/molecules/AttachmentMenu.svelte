<script lang="ts">
  import { FileText, Film, Image, Music, Paperclip } from 'lucide-svelte';
  import * as DropdownMenu from '../atoms/ui/dropdown-menu';
  import { buttonVariants } from '../atoms/ui/button';
  import {
    attachmentAccept,
    attachmentCapabilities,
    type AttachmentCategory,
  } from '../atoms/attachments';
  import type { Model } from '../atoms/types';

  let {
    model,
    protocol,
    disabled = false,
    onFiles,
  }: {
    model?: Model;
    protocol?: string;
    disabled?: boolean;
    onFiles: (files: File[]) => void;
  } = $props();

  let fileInput = $state<HTMLInputElement | null>(null);
  let category = $state<AttachmentCategory | undefined>();
  const accept = $derived(attachmentAccept(model, protocol, category));
  const capabilities = $derived(attachmentCapabilities(model, protocol));
  const options = [
    { category: 'image', label: 'Images', icon: Image },
    { category: 'document', label: 'Documents', icon: FileText },
    { category: 'video', label: 'Video', icon: Film },
    { category: 'audio', label: 'Audio', icon: Music },
  ] as const;

  function openFilePicker(selected: AttachmentCategory) {
    const filter = attachmentAccept(model, protocol, selected);
    if (disabled || !fileInput || !filter) return;
    category = selected;
    // Apply the filter synchronously to keep the browser's user-gesture permission.
    fileInput.accept = filter;
    fileInput.value = '';
    fileInput.click();
  }
</script>

<input
  bind:this={fileInput}
  type="file"
  multiple
  {accept}
  class="sr-only"
  tabindex="-1"
  aria-label="Attach files"
  {disabled}
  onchange={(event) => {
    const files = Array.from(event.currentTarget.files ?? []);
    if (!disabled && files.length) onFiles(files);
    event.currentTarget.value = '';
  }}
/>
<DropdownMenu.Root>
  <DropdownMenu.Trigger
    class={buttonVariants({
      variant: 'ghost',
      size: 'icon',
      class: 'icon-button shrink-0 text-muted-foreground hover:text-foreground',
    })}
    aria-label="Upload attachments"
    title="Add attachments"
    {disabled}
  >
    <Paperclip class="size-4" />
  </DropdownMenu.Trigger>
  <DropdownMenu.Content
    side="top"
    align="start"
    sideOffset={8}
    class="w-64 max-w-[calc(100vw-1rem)]"
    aria-label="Attachment types"
  >
    <DropdownMenu.Label class="text-xs text-muted-foreground">Add attachments</DropdownMenu.Label>
    {#each options as option (option.category)}
      {@const supported = Boolean(attachmentAccept(model, protocol, option.category))}
      <DropdownMenu.Item
        class="min-h-11 gap-2"
        aria-label={option.label}
        disabled={!supported}
        onSelect={() => openFilePicker(option.category)}
      >
        <option.icon class="size-4 shrink-0" />
        <span class="flex-1">{option.label}</span>
        {#if !supported}<span class="text-[11px] text-muted-foreground">Not supported</span>
        {:else if option.category === 'document' && !capabilities.file}<span
            class="text-[11px] text-muted-foreground">Text files only</span
          >{/if}
      </DropdownMenu.Item>
    {/each}
    <DropdownMenu.Separator />
    <p class="px-2 py-1.5 text-[11px] text-muted-foreground">Up to 8 files · 10 MB total</p>
  </DropdownMenu.Content>
</DropdownMenu.Root>
