<script lang="ts">
  import type { ToolActivity } from '../atoms/transcript';
  import ToolActivitySection from '../molecules/ToolActivitySection.svelte';
  import LiveProcessOutput from './LiveProcessOutput.svelte';
  import type { SessionProcesses } from './session-processes.svelte';
  let {
    activity,
    messageID,
    processes,
    running = false,
  }: {
    activity: ToolActivity;
    messageID: string;
    processes: SessionProcesses;
    running?: boolean;
  } = $props();
  const record = $derived(
    activity.processRecord ??
      (activity.call.Name === 'bash' ? processes.forCall(messageID, activity.call.ID) : null),
  );
</script>

<ToolActivitySection
  {activity}
  {running}
  status={record
    ? record.Status === 'running'
      ? 'Running'
      : record.Exit
        ? `Exited ${record.Exit.Code}`
        : 'Stopped'
    : activity.processNotice?.kind === 'exit'
      ? 'Stopped'
      : undefined}
  renderOutput={record && processes.api ? terminal : undefined}
/>
{#snippet terminal()}
  {#if record && processes.api}<LiveProcessOutput api={processes.api} {record} {activity} />{/if}
{/snippet}
