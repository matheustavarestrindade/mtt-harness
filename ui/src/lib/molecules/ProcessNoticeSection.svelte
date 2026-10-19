<script lang="ts">
  import { ChevronRight } from 'lucide-svelte';
  import type { ProcessNotice } from '../atoms/process-notices';
  import type { ProcessRecord } from '../atoms/processes';
  import { shortID } from '../atoms/format';
  import TerminalOutput from './content/TerminalOutput.svelte';
  let { notice, record }: { notice: ProcessNotice; record?: ProcessRecord } = $props();
  let open = $state(false);
  const identifier = $props.id();
</script>

<section
  class="assistant-section tool-activity min-w-0"
  role="group"
  aria-label="Process output"
  data-process-id={notice.processID}
>
  <button
    class="activity-toggle w-full"
    aria-expanded={open}
    aria-controls={identifier}
    onclick={() => (open = !open)}
  >
    <ChevronRight class="size-3.5 shrink-0 text-tool-call-accent {open ? 'rotate-90' : ''}" />
    <span class="min-w-0 flex-1 truncate font-mono">Process {shortID(notice.processID)}</span>
    <span class="text-[10px] text-muted-foreground"
      >{record?.Exit
        ? `Exited ${record.Exit.Code}`
        : notice.kind === 'exit'
          ? 'Stopped'
          : notice.kind === 'inactivity'
            ? 'No output'
            : 'Output'}</span
    >
  </button>
  {#if open}<div
      id={identifier}
      class="tool-details mt-2 min-w-0 border border-border bg-card/70 p-3"
    >
      <TerminalOutput text={notice.output} />
    </div>{/if}
</section>
