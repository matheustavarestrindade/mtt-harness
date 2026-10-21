<script lang="ts">
  import Disclosure from './Disclosure.svelte';
  import type { MemoryAgentUsage } from '../atoms/memory';
  import { agentTokenCount, durationLabel, memoryAgentLabel } from '../atoms/memory';
  import { costLabel, formatTokenCount } from '../atoms/format';
  let { agent, label }: { agent: MemoryAgentUsage; label?: string } = $props();
  const usage = $derived(agent.Statistics);
  const estimated = $derived(usage.Cost?.Estimated || usage.Costs?.some((cost) => cost.Estimated));
</script>

<Disclosure
  class="group border-b border-border/60 last:border-0"
  triggerClass="min-h-14 py-3 text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
>
  {#snippet heading()}
    <span class="min-w-0 flex-1">
      <span class="block font-medium">{label ?? memoryAgentLabel(agent.Agent)}</span>
      <span class="mt-1 block break-all font-mono text-[10px] text-muted-foreground"
        >{agent.ModelID}</span
      >
    </span>
    <span class="shrink-0 text-right tabular-nums">
      <span class="block" title={estimated ? 'Estimated cost' : 'Recorded cost'}
        >{estimated ? '≈ ' : ''}{costLabel(usage)}</span
      >
      <span
        class="mt-1 block text-[10px] text-muted-foreground"
        title="Uncached input + output. Cache is shown separately; output includes reasoning."
        >{formatTokenCount(agentTokenCount(usage))} tokens</span
      >
    </span>
  {/snippet}
  <dl class="grid grid-cols-2 gap-x-3 gap-y-2 pb-3 pl-5 text-[11px] tabular-nums">
    {#each [['Model calls', usage.Calls], ['Input tokens', usage.Input], ['Output tokens', usage.Output], ['Cache read', usage.CacheRead], ['Cache write', usage.CacheWrite], ['Reasoning (included)', usage.Reasoning], ['Failed calls', agent.FailedCalls]] as [label, value]}
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
</Disclosure>
