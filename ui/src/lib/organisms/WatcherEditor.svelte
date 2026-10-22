<script lang="ts">
  import { untrack } from 'svelte';
  import { Button } from '../atoms/ui/button';
  import { Input } from '../atoms/ui/input';
  import { Label } from '../atoms/ui/label';
  import { Switch } from '../atoms/ui/switch';
  import { Textarea } from '../atoms/ui/textarea';
  import type { Model } from '../atoms/types';
  import type { WatcherDefinition, WatcherTool, WatcherPreview } from '../atoms/watcher';
  import { watcherTriggers } from '../atoms/watcher';
  import ModelCombobox from '../molecules/ModelCombobox.svelte';
  import ReasoningSelect from '../molecules/ReasoningSelect.svelte';
  import WatcherToolSelector from '../molecules/WatcherToolSelector.svelte';
  import MultiSelectField from '../molecules/MultiSelectField.svelte';
  import FieldHelp from '../molecules/FieldHelp.svelte';
  import Disclosure from '../molecules/Disclosure.svelte';
  import WatcherReviewInput from '../molecules/WatcherReviewInput.svelte';
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
  class="min-w-0 space-y-6 border-y border-border py-5"
  aria-label="Watcher editor"
  onsubmit={(event) => {
    event.preventDefault();
    void save();
  }}
>
  <fieldset class="min-w-0 space-y-4" disabled={saving}>
    <legend class="mb-3 text-sm font-medium">General</legend>
    <div class="flex min-h-11 items-center justify-between gap-3">
      <Label for={`${identifier}-enabled`}>Enabled</Label>
      <Switch
        id={`${identifier}-enabled`}
        aria-label="Watcher enabled"
        bind:checked={draft.enabled}
        disabled={saving}
      />
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
  </fieldset>
  <fieldset class="min-w-0 space-y-4 border-t border-border pt-5" disabled={saving}>
    <legend class="pr-2 text-sm font-medium">When to run</legend>
    <div class="space-y-2">
      <Label for={`${identifier}-triggers`}>Run on</Label>
      <MultiSelectField
        id={`${identifier}-triggers`}
        value={draft.triggers}
        options={watcherTriggers}
        onValueChange={(selected) =>
          (draft.triggers = watcherTriggers
            .filter((trigger) => selected.includes(trigger.value))
            .map((trigger) => trigger.value))}
        placeholder="Select events"
        required
        disabled={saving}
      />
      <p class="text-xs text-muted-foreground">Select one or more events.</p>
    </div>
    {#if draft.triggers.includes('context_growth')}
      <div class="space-y-2">
        <div class="flex items-center gap-1">
          <Label for={`${identifier}-interval`}>Context interval (tokens)</Label>
          <FieldHelp
            label="About context interval"
            text="How often this watcher runs as visible request content grows: messages, tool data and media. Thinking and private reasoning state never count, even when thinking is included in the review input. For example, 32,768 means one check after each additional 32k visible-context tokens. The model's counter or estimate is used."
          />
        </div>
        <Input
          id={`${identifier}-interval`}
          type="number"
          min={256}
          max={1048576}
          step={1}
          bind:value={draft.interval_tokens}
          disabled={saving}
          required
        />
      </div>
    {/if}
    {#if draft.triggers.includes('permission_request')}
      <div class="flex min-h-11 items-center justify-between gap-4">
        <Label for={`${identifier}-outside`} class="leading-relaxed"
          >Only paths outside the workspace</Label
        >
        <Switch
          id={`${identifier}-outside`}
          bind:checked={draft.outside_workspace_only}
          disabled={saving}
        />
      </div>
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
    {/if}
  </fieldset>
  <fieldset class="min-w-0 space-y-4 border-t border-border pt-5" disabled={saving}>
    <legend class="pr-2 text-sm font-medium">Review input</legend>
    <div class="space-y-2">
      <div class="flex items-center gap-1">
        <Label for={`${identifier}-context`}>Conversation to review (tokens)</Label>
        <FieldHelp
          label="About conversation to review"
          text={"The ceiling for {last_context}, using the model's counter or estimate after the thinking option is applied. Whole messages may use less. Instructions and tools are extra. An explicit {last_8k_context} has its own limit; Context interval controls when reviews run."}
        />
      </div>
      <Input
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
    <div class="flex min-h-11 items-center justify-between gap-4">
      <div class="flex min-w-0 items-center gap-1">
        <Label for={`${identifier}-thinking`} class="leading-relaxed"
          >Include provider-exposed thinking</Label
        >
        <FieldHelp
          label="About thinking in review input"
          text="Adds available thinking text to the reviewer's reference input. This does not change the context-growth trigger: thinking is always excluded from that count."
        />
      </div>
      <Switch
        id={`${identifier}-thinking`}
        bind:checked={draft.include_reasoning}
        disabled={saving}
      />
    </div>
    <div class="space-y-2">
      <Label for={`${identifier}-prompt`}>Prompt</Label><Textarea
        id={`${identifier}-prompt`}
        class="min-h-64 font-mono text-xs leading-5"
        bind:value={draft.prompt}
        maxlength={65536}
        disabled={saving}
        required
      />
      <p class="break-words text-xs text-muted-foreground">
        {'Variables: {todo}, {doing}, {event}, {workspace}, {permission_request}, {tool_call}, {last_context}, {last_32k_context}. k = 1024 tokens.'}
      </p>
    </div>
  </fieldset>
  <fieldset class="min-w-0 space-y-4 border-t border-border pt-5" disabled={saving}>
    <legend class="pr-2 text-sm font-medium">Tool access</legend>
    <WatcherToolSelector
      {tools}
      grants={draft.tools}
      onChange={(value) => (draft.tools = value)}
      disabled={saving}
    />
  </fieldset>
  <fieldset class="min-w-0 space-y-4 border-t border-border pt-5" disabled={saving}>
    <legend class="pr-2 text-sm font-medium">Execution limits</legend>
    <div class="grid gap-4 sm:grid-cols-2">
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
        <div class="flex items-center gap-1">
          <Label for={`${identifier}-output`}>Output budget (tokens)</Label><FieldHelp
            label="About output budget"
            text="Tokens reserved for each watcher response within its model's context limit."
          />
        </div>
        <Input
          id={`${identifier}-output`}
          type="number"
          min={128}
          max={65536}
          bind:value={draft.max_output_tokens}
          required
        />
      </div>
      <div class="space-y-2">
        <div class="flex items-center gap-1">
          <Label for={`${identifier}-continuations`}>Maximum automatic follow-ups</Label><FieldHelp
            label="About automatic follow-ups"
            text="How many times this watcher can ask the main session to continue for the same user instruction. New user input resets the count."
          />
        </div>
        <Input
          id={`${identifier}-continuations`}
          type="number"
          min={1}
          max={100}
          bind:value={draft.max_continuations}
          required
        />
      </div>
    </div>
  </fieldset>
  {#if error}<p role="alert" class="break-words text-sm text-destructive">{error}</p>{/if}
  {#if preview}<Disclosure
      open
      class="rounded border border-border p-3"
      title={`Input preview · ${preview.estimated ? '≈ ' : ''}${preview.input_tokens.toLocaleString()} tokens${preview.truncated ? ' · excerpt' : ''}`}
    >
      <div class="mt-2">
        <WatcherReviewInput input={preview.prompt} truncated={preview.truncated} source="draft" />
      </div>
    </Disclosure>{/if}
  <div class="flex flex-wrap gap-2 border-t border-border pt-4">
    <Button type="submit" class="h-11" disabled={saving}
      >{saving ? 'Saving…' : 'Save watcher settings'}</Button
    ><Button
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
