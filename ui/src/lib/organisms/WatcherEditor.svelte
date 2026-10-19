<script lang="ts">
  import { untrack } from 'svelte';
  import { Button } from '../atoms/ui/button';
  import { Input } from '../atoms/ui/input';
  import { Label } from '../atoms/ui/label';
  import type { Model } from '../atoms/types';
  import type { WatcherDefinition, WatcherTool, WatcherPreview } from '../atoms/watcher';
  import { watcherTriggers } from '../atoms/watcher';
  import ModelCombobox from '../molecules/ModelCombobox.svelte';
  import ReasoningSelect from '../molecules/ReasoningSelect.svelte';
  import WatcherToolSelector from '../molecules/WatcherToolSelector.svelte';
  let {
    value,
    models,
    tools,
    saving = false,
    canPreview = false,
    onSave,
    onCancel,
    onPreview,
  }: {
    value: WatcherDefinition;
    models: Model[];
    tools: WatcherTool[];
    saving?: boolean;
    canPreview?: boolean;
    onSave: (value: WatcherDefinition) => Promise<boolean>;
    onCancel: () => void;
    onPreview: (value: WatcherDefinition) => Promise<WatcherPreview | null>;
  } = $props();
  let draft = $state<WatcherDefinition>(structuredClone($state.snapshot(untrack(() => value))));
  let error = $state(''),
    preview = $state<WatcherPreview | null>(null),
    previewing = $state(false);
  const identifier = $props.id();
  const model = $derived(models.find((model) => model.ID === draft.model));
  async function save() {
    error = '';
    if (!draft.name.trim() || !draft.model || !draft.triggers.length) {
      error = 'Set a name, model, and at least one trigger.';
      return;
    }
    if (
      draft.tools.some(
        (grant) =>
          tools.find((tool) => tool.name === grant.name)?.operations?.length &&
          !grant.operations?.length,
      )
    ) {
      error = 'Select at least one operation for each enabled operation-based tool.';
      return;
    }
    await onSave($state.snapshot(draft));
  }
  async function renderPreview() {
    previewing = true;
    error = '';
    try {
      preview = await onPreview($state.snapshot(draft));
    } catch (failure) {
      error = failure instanceof Error ? failure.message : 'Cannot preview watcher input.';
    } finally {
      previewing = false;
    }
  }
</script>

<form
  class="min-w-0 space-y-5 rounded-md border border-border p-4"
  aria-label="Watcher editor"
  onsubmit={(event) => {
    event.preventDefault();
    void save();
  }}
>
  <div class="flex items-center justify-between gap-3">
    <h3 class="font-medium">Watcher configuration</h3>
    <label class="flex min-h-11 items-center gap-2 text-xs"
      ><input
        type="checkbox"
        class="size-4 accent-foreground"
        bind:checked={draft.enabled}
        disabled={saving}
      />Enabled</label
    >
  </div>
  <div class="space-y-2">
    <Label for={`${identifier}-name`}>Name</Label><Input
      id={`${identifier}-name`}
      bind:value={draft.name}
      maxlength={200}
      disabled={saving}
      required
    />
  </div>
  <div class="space-y-2">
    <Label for={`${identifier}-model`}>Watcher model</Label><ModelCombobox
      id={`${identifier}-model`}
      {models}
      value={draft.model}
      onValueChange={(value) => {
        if (draft.model !== value) draft.reasoning_effort = '';
        draft.model = value;
      }}
      disabled={saving}
      placeholder="Choose a model"
    />
  </div>
  <ReasoningSelect
    {model}
    value={draft.reasoning_effort}
    onChange={(value) => (draft.reasoning_effort = value)}
    stacked
    disabled={saving}
  />
  <fieldset class="space-y-1" disabled={saving}>
    <legend class="mb-2 text-sm font-medium">Run on</legend>{#each watcherTriggers as trigger}<label
        class="flex min-h-11 items-center gap-3 text-sm"
        ><input
          type="checkbox"
          class="size-4 accent-foreground"
          checked={draft.triggers.includes(trigger.value)}
          onchange={(event) =>
            (draft.triggers = event.currentTarget.checked
              ? [...draft.triggers, trigger.value]
              : draft.triggers.filter((value) => value !== trigger.value))}
        />{trigger.label}</label
      >{/each}
  </fieldset>
  <div class="grid min-w-0 gap-4 sm:grid-cols-2">
    {#if draft.triggers.includes('context_growth')}<div class="space-y-2">
        <Label for={`${identifier}-interval`}>Context interval (tokens)</Label><Input
          id={`${identifier}-interval`}
          type="number"
          min={256}
          max={1048576}
          step={1}
          bind:value={draft.interval_tokens}
          disabled={saving}
          required
        />
      </div>{/if}
    <div class="space-y-2">
      <Label for={`${identifier}-context`}>Default context window (tokens)</Label><Input
        id={`${identifier}-context`}
        type="number"
        min={256}
        max={1048576}
        step={1}
        bind:value={draft.context_tokens}
        disabled={saving}
        required
      />
    </div>
  </div>
  <label class="flex min-h-11 items-center gap-3 text-xs"
    ><input
      type="checkbox"
      class="size-4 accent-foreground"
      bind:checked={draft.include_reasoning}
      disabled={saving}
    />Include provider-exposed thinking</label
  >
  {#if draft.triggers.includes('permission_request')}
    <fieldset class="space-y-3 rounded border border-border p-3" disabled={saving}>
      <legend class="px-1 text-xs">Permission filter</legend>
      <label class="flex min-h-11 items-center gap-3 text-xs"
        ><input
          type="checkbox"
          class="size-4 accent-foreground"
          bind:checked={draft.outside_workspace_only}
        />Only paths outside the workspace</label
      >
      <div class="space-y-2">
        <Label for={`${identifier}-permission-tools`}>Tool names (optional, comma-separated)</Label
        ><Input
          id={`${identifier}-permission-tools`}
          value={draft.permission_tools.join(', ')}
          onchange={(event) =>
            (draft.permission_tools = event.currentTarget.value
              .split(',')
              .map((value) => value.trim())
              .filter(Boolean))}
          placeholder="file_actions"
        />
      </div>
    </fieldset>
  {/if}
  <div class="space-y-2">
    <Label for={`${identifier}-prompt`}>Prompt</Label><textarea
      id={`${identifier}-prompt`}
      class="min-h-64 w-full rounded-md border border-input bg-transparent p-3 font-mono text-xs leading-5 outline-none focus-visible:ring-2 focus-visible:ring-ring"
      bind:value={draft.prompt}
      maxlength={65536}
      disabled={saving}
      required></textarea>
    <p class="break-words text-xs text-muted-foreground">
      {'Variables: {todo}, {doing}, {event}, {workspace}, {permission_request}, {tool_call}, {last_context}, {last_32k_context}. k = 1024 tokens.'}
    </p>
  </div>
  <WatcherToolSelector
    {tools}
    grants={draft.tools}
    onChange={(value) => (draft.tools = value)}
    disabled={saving}
  />
  <details class="rounded border border-border px-3">
    <summary class="cursor-pointer py-3 text-xs">Execution limits</summary>
    <div class="grid gap-4 pb-4 sm:grid-cols-2">
      <div class="space-y-2">
        <Label for={`${identifier}-timeout`}>Review timeout (ms)</Label><Input
          id={`${identifier}-timeout`}
          type="number"
          min={1000}
          max={900000}
          bind:value={draft.timeout_ms}
          required
        />
      </div>
      <div class="space-y-2">
        <Label for={`${identifier}-rounds`}>Maximum model rounds</Label><Input
          id={`${identifier}-rounds`}
          type="number"
          min={1}
          max={100}
          bind:value={draft.max_rounds}
          required
        />
      </div>
      <div class="space-y-2">
        <Label for={`${identifier}-output`}>Output reservation (tokens)</Label><Input
          id={`${identifier}-output`}
          type="number"
          min={128}
          max={65536}
          bind:value={draft.max_output_tokens}
          required
        />
      </div>
      <div class="space-y-2">
        <Label for={`${identifier}-continuations`}>Maximum automatic continuations</Label><Input
          id={`${identifier}-continuations`}
          type="number"
          min={1}
          max={100}
          bind:value={draft.max_continuations}
          required
        />
      </div>
    </div>
  </details>
  {#if error}<p role="alert" class="break-words text-sm text-destructive">{error}</p>{/if}
  {#if preview}<details open class="min-w-0 rounded border border-border p-3">
      <summary class="cursor-pointer text-xs"
        >Input preview · {preview.estimated ? '≈ ' : ''}{preview.input_tokens.toLocaleString()} tokens{preview.truncated
          ? ' · excerpt'
          : ''}</summary
      >
      <pre
        class="mt-3 max-h-72 overflow-auto whitespace-pre-wrap break-words text-xs">{preview.prompt}</pre>
    </details>{/if}
  <div class="flex flex-wrap gap-2">
    <Button type="submit" class="h-11" disabled={saving}>Save watcher</Button><Button
      type="button"
      variant="outline"
      class="h-11"
      disabled={saving || previewing || !canPreview}
      onclick={renderPreview}>Preview input</Button
    ><Button type="button" variant="ghost" class="h-11" disabled={saving} onclick={onCancel}
      >Cancel</Button
    >
  </div>
</form>
