<script lang="ts">
  import { Settings } from 'lucide-svelte';
  import { Switch } from '../atoms/ui/switch';
  import { Button } from '../atoms/ui/button';

  let {
    enabled,
    status,
    toggleLabel,
    configureLabel = 'Configure',
    disabled = false,
    toggleDisabled = false,
    expanded = false,
    onToggle,
    onConfigure,
  }: {
    enabled: boolean;
    status: string;
    toggleLabel: string;
    configureLabel?: string;
    disabled?: boolean;
    toggleDisabled?: boolean;
    expanded?: boolean;
    onToggle: (enabled: boolean) => void;
    onConfigure?: () => void;
  } = $props();
</script>

<div data-plugin-header-controls class="flex min-h-11 shrink-0 items-center gap-2">
  <span role="status" class="text-xs text-muted-foreground">{status}</span>
  <Switch
    aria-label={toggleLabel}
    disabled={disabled || toggleDisabled}
    bind:checked={() => enabled, onToggle}
  />
  {#if onConfigure}<Button
      type="button"
      variant="ghost"
      size="icon"
      class="icon-button shrink-0"
      aria-label={configureLabel}
      title={configureLabel}
      aria-expanded={expanded}
      {disabled}
      onclick={onConfigure}><Settings class="size-4" /></Button
    >{/if}
</div>
