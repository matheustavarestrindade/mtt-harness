<script lang="ts">
  let { active = false, label = 'Inactive' }: { active?: boolean; label?: string } = $props();
</script>

<span
  role="img"
  aria-label={label}
  title={label}
  data-slot="agent-status"
  data-active={active}
  class="relative inline-flex size-2 shrink-0 rounded-full {active
    ? 'bg-foreground'
    : 'bg-muted-foreground/40'}"
>
  {#if active}<span
      class="absolute inset-0 rounded-full bg-current opacity-50 motion-safe:animate-ping"
      aria-hidden="true"
    ></span>{/if}
</span>
