<script lang="ts">
  import { Ellipsis } from 'lucide-svelte';
  import * as DropdownMenu from '../atoms/ui/dropdown-menu';
  import { buttonVariants } from '../atoms/ui/button';
  let {
    label = 'Content actions',
    actions,
  }: { label?: string; actions: { label: string; select: () => void | Promise<void> }[] } =
    $props();
</script>

<DropdownMenu.Root>
  <DropdownMenu.Trigger
    aria-label={label}
    title={label}
    class={buttonVariants({
      variant: 'ghost',
      size: 'icon',
      class: 'size-7 shrink-0 text-muted-foreground pointer-coarse:size-11',
    })}
  >
    <Ellipsis class="size-3.5" />
  </DropdownMenu.Trigger>
  <DropdownMenu.Content align="end" aria-label={label} class="max-w-[calc(100vw-1rem)]">
    {#each actions as action}<DropdownMenu.Item
        class="min-h-9 pointer-coarse:min-h-11"
        onSelect={() => void action.select()}>{action.label}</DropdownMenu.Item
      >{/each}
  </DropdownMenu.Content>
</DropdownMenu.Root>
