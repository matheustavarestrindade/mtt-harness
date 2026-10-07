<script lang="ts">
  import { onDestroy } from 'svelte';
  import { LoaderCircle } from 'lucide-svelte';
  import { toast } from 'svelte-sonner';
  import { Button } from '../atoms/ui/button';
  import { Input } from '../atoms/ui/input';
  import { Label } from '../atoms/ui/label';
  import type { Instance } from '../atoms/types';
  import { workspaceName } from '../atoms/format';
  import {
    runtimeSettingFields,
    type RuntimeSettingKey,
    type RuntimeSettings,
  } from '../atoms/settings';
  import type { HarnessApi } from '../molecules/api/client';
  import SelectField from '../molecules/SelectField.svelte';

  let { api, instance }: { api: HarnessApi; instance: Instance | null } = $props();
  let scope = $state('harness');
  let revision = $state(0);
  let values = $state<RuntimeSettings>({});
  let saved = $state<RuntimeSettings>({});
  let globalValues = $state<RuntimeSettings>({});
  let loading = $state(true);
  let error = $state('');
  let busy = $state('');
  let fieldErrors = $state<RuntimeSettings>({});
  const lifetime = new AbortController();
  const instanceID = $derived(scope === 'workspace' ? (instance?.ID ?? '') : '');
  const validLimit = (value: string) =>
    !value.trim() || (/^\d+$/.test(value.trim()) && Number.isSafeInteger(Number(value)));

  $effect(() => {
    const identifier = instanceID;
    const currentAPI = api;
    revision;
    const controller = new AbortController();
    const signal = AbortSignal.any([controller.signal, lifetime.signal]);
    loading = true;
    error = '';
    fieldErrors = {};
    void Promise.all([
      currentAPI.settings('', signal),
      identifier ? currentAPI.settings(identifier, signal) : Promise.resolve(null),
    ])
      .then(([globalSettings, workspaceSettings]) => {
        if (signal.aborted) return;
        globalValues = globalSettings;
        saved = workspaceSettings ?? globalSettings;
        values = { ...saved };
      })
      .catch((failure: unknown) => {
        if (!signal.aborted)
          error = failure instanceof Error ? failure.message : 'Cannot load settings.';
      })
      .finally(() => {
        if (!signal.aborted) loading = false;
      });
    return () => controller.abort();
  });

  async function saveRuntimeSetting(key: RuntimeSettingKey) {
    const value = (values[key] ?? '').trim();
    if (!validLimit(value) || busy) return;
    const identifier = instanceID;
    busy = key;
    fieldErrors[key] = '';
    try {
      if (value) await api.saveSetting(identifier, key, value, lifetime.signal);
      else await api.deleteSetting(identifier, key, lifetime.signal);
      if (lifetime.signal.aborted) return;
      saved[key] = value || undefined;
      values[key] = value;
      if (!identifier) globalValues[key] = value || undefined;
      toast.success('Setting saved');
    } catch (failure) {
      if (!lifetime.signal.aborted)
        fieldErrors[key] =
          failure instanceof Error ? failure.message : 'Could not save the setting.';
    } finally {
      if (!lifetime.signal.aborted) busy = '';
    }
  }
  function updateLimitInput(key: RuntimeSettingKey, event: Event) {
    if (!(event.currentTarget instanceof HTMLInputElement)) return;
    values[key] = event.currentTarget.value;
  }
  function inheritedLabel(key: RuntimeSettingKey): string {
    if (instanceID && globalValues[key] !== undefined)
      return `Harness setting: ${globalValues[key]}`;
    const configured =
      key === 'agent_depth_limit' ? instance?.AgentDepthLimit : instance?.ProcessLimit;
    if (instanceID && configured && configured > 0) return `Workspace configuration: ${configured}`;
    return 'Harness default';
  }
  onDestroy(() => lifetime.abort());
</script>

<section aria-label="Runtime settings" class="mx-auto w-full max-w-2xl">
  <h2 class="text-xl font-medium tracking-tight">General</h2>
  <p class="mt-2 text-sm leading-6 text-muted-foreground">
    Set limits for the harness or the selected workspace. Workspace settings take priority.
  </p>
  <div class="my-6 space-y-2">
    <Label for="settings-scope">Settings scope</Label>
    <SelectField
      id="settings-scope"
      bind:value={scope}
      disabled={!!busy}
      options={[
        { value: 'harness', label: 'Entire harness' },
        ...(instance
          ? [{ value: 'workspace', label: `Workspace · ${workspaceName(instance.Workspace)}` }]
          : []),
      ]}
    />
  </div>
  {#if loading}<p role="status" class="flex items-center gap-2 text-sm text-muted-foreground">
      <LoaderCircle class="size-4 animate-spin" />Loading settings…
    </p>
  {:else if error}
    <p role="alert" class="break-words text-sm text-destructive">{error}</p>
    <Button variant="outline" class="mt-4 h-11" onclick={() => revision++}>Retry</Button>
  {:else}
    <div class="space-y-6">
      {#each runtimeSettingFields as field (field.key)}
        <form
          class="rounded-xl border border-border p-4 sm:p-5"
          onsubmit={(event) => {
            event.preventDefault();
            void saveRuntimeSetting(field.key);
          }}
        >
          <Label for={`setting-${field.key}`}>{field.label}</Label>
          <p
            id={`setting-description-${field.key}`}
            class="mt-2 text-xs leading-5 text-muted-foreground"
          >
            {field.description}
          </p>
          <div class="mt-4 flex items-center gap-3">
            <Input
              id={`setting-${field.key}`}
              type="number"
              min="0"
              step="1"
              inputmode="numeric"
              value={values[field.key] ?? ''}
              oninput={(event: Event) => updateLimitInput(field.key, event)}
              placeholder="Use default"
              class="h-11 min-w-0"
              disabled={!!busy}
              aria-describedby={`setting-description-${field.key}`}
              aria-invalid={!validLimit(values[field.key] ?? '')}
            />
            <Button
              type="submit"
              class="h-11 shrink-0"
              disabled={!!busy ||
                !validLimit(values[field.key] ?? '') ||
                (values[field.key] ?? '').trim() === (saved[field.key] ?? '')}
            >
              {#if busy === field.key}<LoaderCircle class="size-4 animate-spin" />{/if}Save
            </Button>
          </div>
          <p class="mt-3 text-xs leading-5 text-muted-foreground">
            Leave empty to use the default. {inheritedLabel(field.key)}.
          </p>
          {#if fieldErrors[field.key]}<p
              role="alert"
              class="mt-3 break-words text-sm text-destructive"
            >
              {fieldErrors[field.key]}
            </p>{/if}
        </form>
      {/each}
    </div>
  {/if}
</section>
