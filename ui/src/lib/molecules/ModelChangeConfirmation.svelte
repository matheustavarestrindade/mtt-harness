<script lang="ts">
  import * as Dialog from '../atoms/ui/dialog';
  import { Button } from '../atoms/ui/button';
  let {
    model = '',
    currentContext = 0,
    targetContext = 0,
    busy = false,
    onConfirm,
    onCancel,
  }: {
    model?: string;
    currentContext?: number;
    targetContext?: number;
    busy?: boolean;
    onConfirm: () => void;
    onCancel: () => void;
  } = $props();
</script>

<Dialog.Header>
  <Dialog.Title>Compact context and switch model?</Dialog.Title>
  <Dialog.Description>
    {currentContext > 0
      ? 'This model has a smaller context window.'
      : 'The previous context size is unavailable.'}
    The conversation context will be compacted to fit.
  </Dialog.Description>
</Dialog.Header>
<div class="space-y-3 text-sm leading-6">
  <p class="break-words font-medium">{model}</p>
  <p class="text-muted-foreground">
    {currentContext > 0 ? currentContext.toLocaleString() : 'Unknown'} → {targetContext.toLocaleString()}
    tokens
  </p>
  <p class="text-muted-foreground">
    Older complete turns may be omitted from the model request. Your saved conversation stays
    available. The current response continues with its original model.
  </p>
</div>
<Dialog.Footer>
  <Button variant="outline" class="h-11" disabled={busy} onclick={onCancel}>Cancel</Button>
  <Button class="h-11" disabled={busy} onclick={onConfirm}
    >{busy ? 'Switching…' : 'Switch and compact'}</Button
  >
</Dialog.Footer>
