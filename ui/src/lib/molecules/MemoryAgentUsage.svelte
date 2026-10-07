<script lang="ts">
  import { ChevronRight } from 'lucide-svelte';
  import type { MemoryAgentUsage } from '../atoms/memory';
  import { agentTokenCount, durationLabel, memoryAgentLabel } from '../atoms/memory';
  import { costLabel, formatTokenCount } from '../atoms/format';
  let { agent }: { agent: MemoryAgentUsage } = $props();
  const usage = $derived(agent.Statistics);
  const estimated = $derived(usage.Cost?.Estimated || usage.Costs?.some((cost) => cost.Estimated));
</script>

<details class="group border-b border-border/60 last:border-0">
  <summary
    class="flex min-h-14 cursor-pointer list-none items-center gap-2 py-3 text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring [&::-webkit-details-marker]:hidden"
  >
    <ChevronRight
      class="size-3 shrink-0 text-muted-foreground transition-transform group-open:rotate-90"
    />
    <span class="min-w-0 flex-1">
      <span class="block font-medium">{memoryAgentLabel(agent.Agent)}</span>
      <span class="mt-1 block break-all font-mono text-[10px] text-muted-foreground"
        >{agent.ModelID}</span
      >
    </span>
    <span class="shrink-0 text-right tabular-nums">
      <span class="block" title={estimated ? 'Estimated cost' : 'Recorded cost'}
        >{estimated ? '≈ ' : ''}{costLabel(usage)}</span
      >
      <span class="mt-1 block text-[10px] text-muted-foreground"
        >{formatTokenCount(agentTokenCount(usage))} tokens</span
      >
    </span>
  </summary>
  <dl class="grid grid-cols-2 gap-x-3 gap-y-2 pb-3 pl-5 text-[11px] tabular-nums">
    {#each [['Model calls', usage.Calls], ['Input tokens', usage.Input + usage.CacheRead + usage.CacheWrite], ['Output tokens', usage.Output], ['Cache read', usage.CacheRead], ['Reasoning tokens', usage.Reasoning], ['Failed calls', agent.FailedCalls]] as [label, value]}
      <div class="min-w-0">
        <dt class="text-muted-foreground">{label}</dt>
        <dd class="mt-0.5 font-medium">{Number(value).toLocaleString()}</dd>
      </div>
    {/each}
    <div>
      <dt class="text-muted-foreground">Request time</dt>
      <dd class="mt-0.5 font-medium">{durationLabel(agent.DurationMilliseconds)}</dd>
    </div>
  </dl>
</details>
