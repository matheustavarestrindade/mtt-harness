<script lang="ts">
  import { onDestroy } from 'svelte';
  import { LoaderCircle, SlidersHorizontal } from 'lucide-svelte';
  import { toast } from 'svelte-sonner';
  import { Button } from '../atoms/ui/button';
  import { Input } from '../atoms/ui/input';
  import { Label } from '../atoms/ui/label';
  import type { Instance } from '../atoms/types';
  import {
    runtimeSettingFields,
    type RuntimeSettingKey,
    type RuntimeSettings,
    type SettingsScope,
  } from '../atoms/settings';
  import type { HarnessApi } from '../molecules/api/client';
  import SettingsPageTitle from '../molecules/SettingsPageTitle.svelte';

  let {
    api,
    instance,
    scope = 'harness',
  }: { api: HarnessApi; instance: Instance | null; scope?: SettingsScope } = $props();
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

<section aria-label="Runtime settings" class="min-w-0 w-full space-y-5">
  <SettingsPageTitle
    icon={SlidersHorizontal}
    title="General"
    description={scope === 'workspace'
      ? 'Set workspace limits. Empty values use the harness defaults.'
      : 'Set default limits for the harness.'}
  />
  {#if loading}<p role="status" class="flex items-center gap-2 text-sm text-muted-foreground">
      <LoaderCircle class="size-4 animate-spin" />Loading settings…
    </p>
  {:else if error}
    <p role="alert" class="break-words text-sm text-destructive">{error}</p>
    <Button variant="outline" class="h-11" onclick={() => revision++}>Retry</Button>
  {:else}
    <div class="divide-y divide-border border-t border-border">
      {#each runtimeSettingFields as field (field.key)}
        <form
          class="min-w-0 space-y-3 py-5 last:pb-0"
          aria-label={`${field.label} settings`}
          onsubmit={(event) => {
            event.preventDefault();
            void saveRuntimeSetting(field.key);
          }}
        >
          <Label id={`setting-label-${field.key}`} for={`setting-${field.key}`}>{field.label}</Label
          >
          <p
            id={`setting-description-${field.key}`}
            class="text-xs leading-5 text-muted-foreground"
          >
            {field.description}
          </p>
          <div class="flex items-center gap-3">
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
          <p class="text-xs leading-5 text-muted-foreground">
            Leave empty to use the default. {inheritedLabel(field.key)}.
          </p>
          {#if fieldErrors[field.key]}<p role="alert" class="break-words text-sm text-destructive">
              {fieldErrors[field.key]}
            </p>{/if}
        </form>
      {/each}
    </div>
  {/if}
</section>
