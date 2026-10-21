<script lang="ts">
  import { GitBranch } from 'lucide-svelte';
  import { agentTask } from '../atoms/agent-activity';
  import type { ToolActivity } from '../atoms/transcript';
  import type { Model, Session } from '../atoms/types';
  import MessageContent from '../molecules/content/MessageContent.svelte';
  import Disclosure from '../molecules/Disclosure.svelte';
  import AgentPanel from './AgentPanel.svelte';
  import type { AgentMonitor } from './agent-monitor.svelte';

  let {
    monitor,
    sessions,
    models,
    activities = [],
    running = false,
  }: {
    monitor: AgentMonitor;
    sessions: Session[];
    models: Model[];
    activities?: ToolActivity[];
    running?: boolean;
  } = $props();
</script>

<section class="assistant-section min-w-0 space-y-2" aria-label="Delegated agents">
  {#if monitor.error}<p role="alert" class="text-xs text-destructive">{monitor.error}</p>{/if}
  {#each sessions as session (session.ID)}<AgentPanel {session} {monitor} {models} />{/each}
  {#if !sessions.length}
    {#each activities as activity (activity.call.ID)}
      <div class="min-w-0 border border-border bg-card/30 p-2">
        <div class="flex items-center gap-2 text-xs">
          <GitBranch class="size-3.5 shrink-0" /><span
            class="min-w-0 truncate"
            title={agentTask(activity.call)}
            >{agentTask(activity.call).split('\n')[0] || 'Delegated task'}</span
          >
        </div>
        {#if activity.result}<MessageContent
            content={activity.result.Content}
            mode="plain"
            embedded
          />
        {:else}<p class="mt-1 text-[10px] text-muted-foreground">
            {running ? 'Waiting for agent details…' : 'No agent run is linked to this call.'}
          </p>{/if}
      </div>
    {/each}
  {/if}
  {#if sessions.length && activities.some((activity) => activity.result)}
    <Disclosure class="text-xs text-muted-foreground" title="Delegation results">
      <div class="max-h-40 space-y-2 overflow-y-auto overscroll-contain">
        {#each activities.filter((activity) => activity.result) as activity (activity.call.ID)}
          <div class="min-w-0 border-l border-border pl-2">
            <p class="mb-1 truncate" title={agentTask(activity.call)}>
              {agentTask(activity.call).split('\n')[0]}
            </p>
            <MessageContent content={activity.result!.Content} mode="plain" embedded />
          </div>
        {/each}
      </div>
    </Disclosure>
  {/if}
</section>
