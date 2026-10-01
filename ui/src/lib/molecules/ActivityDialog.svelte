<script lang="ts">
  import { Radio, Terminal } from 'lucide-svelte';
  import * as Dialog from '$lib/atoms/ui/dialog';
  import type { HarnessEvent } from '$lib/atoms/types';
  import { time } from '$lib/atoms/format';
  let { open = $bindable(false), events = [] }: { open?: boolean; events?: HarnessEvent[] } =
    $props();
</script>

<Dialog.Root bind:open
  ><Dialog.Content class="sm:max-w-xl"
    ><Dialog.Header
      ><Dialog.Title>Session activity</Dialog.Title><Dialog.Description
        >The latest harness events. Message history is loaded from the API.</Dialog.Description
      ></Dialog.Header
    >
    <div class="max-h-[55dvh] space-y-2 overflow-y-auto">
      {#if !events.length}<div class="grid place-items-center gap-3 py-12 text-muted-foreground">
          <Radio class="size-6" />
          <p class="text-sm">No events yet.</p>
        </div>{/if}{#each [...events].reverse() as event (event.Seq)}<details
          class="rounded-lg border border-border"
        >
          <summary class="flex min-h-11 cursor-pointer items-center gap-2 px-3 text-xs"
            ><Terminal class="size-3 text-primary" /><span class="font-mono">{event.Name}</span
            ><span class="ml-auto text-[10px] text-muted-foreground">{time(event.Time)}</span
            ></summary
          >
          <pre class="tool-output m-2 whitespace-pre-wrap break-all">{JSON.stringify(
              event.Payload,
              null,
              2,
            ) ?? 'No payload'}</pre>
        </details>{/each}
    </div></Dialog.Content
  ></Dialog.Root
>
