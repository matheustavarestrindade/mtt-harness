<script lang="ts">
  import { CircleHelp } from 'lucide-svelte';
  import * as Tooltip from '../atoms/ui/tooltip';
  import { buttonVariants } from '../atoms/ui/button';

  let { label, text }: { label: string; text: string } = $props();
  let open = $state(false);
</script>

<Tooltip.Provider delayDuration={150}>
  <Tooltip.Root bind:open disableCloseOnTriggerClick ignoreNonKeyboardFocus>
    <Tooltip.Trigger
      type="button"
      aria-label={label}
      class={buttonVariants({
        variant: 'ghost',
        size: 'icon',
        class: 'size-7 shrink-0 text-muted-foreground pointer-coarse:size-11',
      })}
      onclick={() => (open = true)}><CircleHelp class="size-4" /></Tooltip.Trigger
    >
    <Tooltip.Content
      role="tooltip"
      side="top"
      sideOffset={6}
      collisionPadding={12}
      class="z-[70] max-w-[min(19rem,calc(100vw-2rem))] text-left leading-relaxed"
    >
      {text}
    </Tooltip.Content>
  </Tooltip.Root>
</Tooltip.Provider>
