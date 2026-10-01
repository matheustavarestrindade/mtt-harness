<script lang="ts">
  import { ShieldQuestion, Check, X } from 'lucide-svelte';
  import { Button } from '$lib/atoms/ui/button';
  import type { PermissionRequest } from '$lib/atoms/types';
  let {
    request,
    busy = false,
    onDecision,
  }: {
    request: PermissionRequest;
    busy?: boolean;
    onDecision: (identifier: string, kind: 'allow' | 'deny') => void;
  } = $props();
</script>

<section
  class="rounded-xl border border-amber-300/25 bg-amber-300/5 p-4"
  aria-label="Permission request"
>
  <div class="flex items-start gap-3">
    <ShieldQuestion class="mt-0.5 size-5 shrink-0 text-amber-200" />
    <div class="min-w-0 flex-1">
      <h3 class="text-sm font-medium text-amber-100">Permission needed</h3>
      <p class="mt-2 break-all font-mono text-xs text-foreground">{request.Target}</p>
      <p class="mt-2 text-xs leading-5 text-muted-foreground">
        {request.Why || 'The harness is waiting for your decision.'}
      </p>
    </div>
  </div>
  <div class="mt-4 flex flex-wrap justify-end gap-2">
    <Button
      variant="outline"
      class="h-11"
      disabled={busy}
      onclick={() => onDecision(request.ID, 'deny')}><X class="size-4" />Deny</Button
    ><Button class="h-11" disabled={busy} onclick={() => onDecision(request.ID, 'allow')}
      ><Check class="size-4" />Allow once</Button
    >
  </div>
</section>
