<script lang="ts">
  import { RefreshCw } from 'lucide-svelte';
  import { Button } from '../atoms/ui/button';

  let {
    name,
    updatedAt = null,
    refreshing = false,
    disabled = false,
    onRefresh,
  }: {
    name: string;
    updatedAt?: Date | null;
    refreshing?: boolean;
    disabled?: boolean;
    onRefresh: () => void;
  } = $props();
</script>

<div class="flex items-center justify-between gap-3 border-t border-border pt-3">
  <span class="text-xs text-muted-foreground"
    >{updatedAt ? `Updated ${updatedAt.toLocaleTimeString()}` : 'Workspace activity'}</span
  >
  <Button variant="ghost" class="h-11" disabled={disabled || refreshing} onclick={onRefresh}>
    <RefreshCw
      class={`size-4 ${refreshing ? 'animate-spin motion-reduce:animate-none' : ''}`}
    />Refresh {name}
  </Button>
</div>
