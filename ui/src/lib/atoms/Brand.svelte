<script lang="ts">
  import { Terminal } from 'lucide-svelte';
</script>

<div class="flex items-center gap-3">
  <div
    class="grid size-10 place-items-center rounded-xl border border-primary/20 bg-primary/10 text-primary"
  >
    <Terminal class="size-5" strokeWidth={1.6} />
  </div>
  <div>
    <div class="flex items-center gap-2">
      <span class="text-base font-semibold tracking-tight"
        >mtt<span class="text-muted-foreground">/</span>console</span
      ><span
        class="rounded border border-border px-1.5 py-0.5 text-[9px] font-medium text-muted-foreground"
        >LOCAL</span
      >
    </div>
    <p class="mt-0.5 text-[11px] text-muted-foreground">A window into your harness</p>
  </div>
</div>
