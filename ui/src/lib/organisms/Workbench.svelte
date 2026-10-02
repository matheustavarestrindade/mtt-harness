<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import {
    Menu,
    ChevronRight,
    FolderOpen,
    MessageSquarePlus,
    Radio,
    Settings2,
    CircleAlert,
    LoaderCircle,
  } from 'lucide-svelte';
  import { toast } from 'svelte-sonner';
  import { Toaster } from '$lib/atoms/ui/sonner';
  import { Button } from '$lib/atoms/ui/button';
  import * as Sheet from '$lib/atoms/ui/sheet';
  import Navigation from '$lib/organisms/Navigation.svelte';
  import SettingsDialog from '$lib/organisms/SettingsDialog.svelte';
  import WorkspaceDialog from '$lib/molecules/WorkspaceDialog.svelte';
  import SessionDialog from '$lib/molecules/SessionDialog.svelte';
  import ActivityDialog from '$lib/molecules/ActivityDialog.svelte';
  import Conversation from '$lib/organisms/Conversation.svelte';
  import Composer from '$lib/molecules/Composer.svelte';
  import UsageBar from '$lib/molecules/UsageBar.svelte';
  import SessionSettingsDialog from '$lib/molecules/SessionSettingsDialog.svelte';
  import { findSessionModel } from '$lib/atoms/reasoning';
  import { HarnessConsole } from '$lib/organisms/console.svelte';
  import { ApiError } from '$lib/molecules/api/client';
  import { loadConnection, type ConnectionPreferences } from '$lib/molecules/connection-storage';
  import { shortID, workspaceName } from '$lib/atoms/format';
  import type { Instance, InstanceInput, Session } from '$lib/atoms/types';
  import type { SettingsSection } from '$lib/atoms/settings';

  const workbench = new HarnessConsole();
  let preferences = $state<ConnectionPreferences>({ base: '/api', token: '' });
  let settingsOpen = $state(false);
  let settingsSection = $state<SettingsSection>('general');
  let workspaceOpen = $state(false);
  let workspaceError = $state('');
  let sessionOpen = $state(false);
  let sessionSettingsOpen = $state(false);
  let settingsSessionID = $state('');
  let sessionSettingsError = $state('');
  let sessionSettingsNotice = $state('');
  let pendingModel = $state<{
    sessionID: string;
    model: string;
    currentContext: number;
    targetContext: number;
  } | null>(null);
  let activityOpen = $state(false);
  let menuOpen = $state(false);
  let busy = $state('');
  let sending = $state(false);
  let drafts = $state<Record<string, string>>({});
  const sessionID = $derived(workbench.session?.ID ?? '');
  const sessionModel = $derived(findSessionModel(workbench.models, workbench.session?.Model ?? ''));
  const disabled = $derived(
    workbench.connection !== 'connected' ||
      !workbench.session ||
      !!workbench.instance?.Stopped ||
      !!workbench.session?.Completed,
  );
  const connectionLabel = $derived(
    workbench.connection === 'connected'
      ? 'Connected'
      : workbench.connection === 'connecting'
        ? 'Connecting'
        : 'Connect API',
  );

  function openSettings(section?: SettingsSection) {
    menuOpen = false;
    settingsSection = section ?? (workbench.connection === 'connected' ? 'general' : 'connection');
    settingsOpen = true;
  }

  $effect(() => {
    if (workspaceOpen) workspaceError = '';
  });
  $effect(() => {
    if (sessionSettingsOpen && settingsSessionID !== sessionID) {
      sessionSettingsOpen = false;
      pendingModel = null;
    }
  });
  $effect(() => {
    if (!sessionSettingsOpen) {
      pendingModel = null;
      sessionSettingsError = '';
      sessionSettingsNotice = '';
    }
  });

  async function runWorkbenchAction(name: string, operation: () => Promise<void>) {
    busy = name;
    try {
      await operation();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'The operation failed.';
      toast.error(message);
      if (error instanceof ApiError && error.status === 401) openSettings('connection');
    } finally {
      if (busy === name) busy = '';
    }
  }
  function openSessionSettings() {
    settingsSessionID = sessionID;
    pendingModel = null;
    sessionSettingsError = '';
    sessionSettingsNotice = '';
    sessionSettingsOpen = true;
  }
  async function connect(base: string, token: string) {
    await runWorkbenchAction('connect', async () => {
      await workbench.connect(base, token);
      if (workbench.connection !== 'connected') return;
      preferences = { base, token };
      settingsOpen = false;
      toast.success('Connected to the harness');
    });
  }
  async function createWorkspace(input: InstanceInput) {
    workspaceError = '';
    await runWorkbenchAction('workspace', async () => {
      try {
        await workbench.createInstance(input);
      } catch (error) {
        const missingDirectory =
          error instanceof ApiError &&
          error.status === 400 &&
          /^(?:stat|lstat) .*: no such file or directory$/s.test(error.message);
        workspaceError = missingDirectory
          ? `The directory "${input.workspace}" does not exist on the harness server. Use an existing directory such as /workspace, or create your project folder and make it accessible inside Docker first.`
          : error instanceof Error
            ? error.message
            : 'Could not open the workspace.';
        if (missingDirectory) throw new ApiError(workspaceError, error.status);
        throw error;
      }
      workspaceOpen = false;
      menuOpen = false;
      toast.success('Workspace created');
      if (!workbench.session) sessionOpen = true;
    });
  }
  async function createSession(model: string, effort = '') {
    await runWorkbenchAction('session', async () => {
      await workbench.createSession(model, effort);
      sessionOpen = false;
      menuOpen = false;
      toast.success('Session ready');
    });
  }
  function selectInstance(instance: Instance) {
    menuOpen = false;
    void runWorkbenchAction('select', () => workbench.selectInstance(instance));
  }
  async function changeSessionModel(model: string, allowCompaction = false) {
    const selectedSession = workbench.session;
    if (!selectedSession) return;
    sessionSettingsError = '';
    sessionSettingsNotice = '';
    await runWorkbenchAction('model', async () => {
      try {
        await workbench.setSessionModel(model, allowCompaction);
        if (workbench.session?.ID !== selectedSession.ID) return;
        pendingModel = null;
        sessionSettingsNotice =
          selectedSession.ReasoningEffort && !workbench.session.ReasoningEffort
            ? 'Model updated. Thinking uses the model’s default.'
            : 'Model updated.';
      } catch (failure) {
        if (!sessionSettingsOpen || workbench.session?.ID !== selectedSession.ID) return;
        if (
          failure instanceof ApiError &&
          failure.details.code === 'context_compaction_required' &&
          workbench.session?.ID === selectedSession.ID
        ) {
          pendingModel = {
            sessionID: selectedSession.ID,
            model: typeof failure.details.model === 'string' ? failure.details.model : model,
            currentContext:
              typeof failure.details.current_context_max === 'number'
                ? failure.details.current_context_max
                : 0,
            targetContext:
              typeof failure.details.target_context_max === 'number'
                ? failure.details.target_context_max
                : 0,
          };
          return;
        }
        showSessionSettingsError(failure);
      }
    });
  }
  function showSessionSettingsError(failure: unknown) {
    sessionSettingsError =
      failure instanceof Error ? failure.message : 'Cannot update this session.';
    if (failure instanceof ApiError && failure.status === 401) {
      sessionSettingsOpen = false;
      openSettings('connection');
    }
  }
  async function changeSessionEffort(effort: string) {
    sessionSettingsError = '';
    sessionSettingsNotice = '';
    await runWorkbenchAction('reasoning', async () => {
      try {
        await workbench.setReasoningEffort(effort);
        sessionSettingsNotice = 'Thinking updated.';
      } catch (failure) {
        showSessionSettingsError(failure);
      }
    });
  }
  function selectSession(session: Session) {
    menuOpen = false;
    void runWorkbenchAction('select', () => workbench.selectSession(session));
  }
  async function sendDraftMessage() {
    const identifier = sessionID;
    const content = drafts[identifier] ?? '';
    if (disabled || sending || !content.trim()) return;
    sending = true;
    try {
      await workbench.sendMessage(content);
      if (drafts[identifier] === content) drafts[identifier] = '';
    } catch (error) {
      toast.error(error instanceof Error ? error.message : 'Message was not accepted.');
    } finally {
      sending = false;
    }
  }
  function resolvePermissionRequest(identifier: string, kind: 'allow' | 'deny') {
    void runWorkbenchAction(identifier, async () => {
      try {
        await workbench.resolvePermissionRequest(identifier, kind);
      } catch (error) {
        if (error instanceof ApiError && error.status === 404)
          workbench.permissions = workbench.permissions.filter((entry) => entry.ID !== identifier);
        throw error;
      }
      toast.success(kind === 'allow' ? 'Permission allowed once' : 'Permission denied');
    });
  }
  onMount(() => {
    preferences = loadConnection();
    if (preferences.token) void connect(preferences.base, preferences.token);
  });
  onDestroy(() => workbench.disconnect(false));
</script>

{#snippet navigation()}
  <Navigation
    console={workbench}
    onSettings={() => openSettings()}
    onWorkspace={() => {
      menuOpen = false;
      workspaceOpen = true;
    }}
    onSession={() => {
      menuOpen = false;
      sessionOpen = true;
    }}
    onSelectInstance={selectInstance}
    onSelectSession={selectSession}
    onRefresh={() => void runWorkbenchAction('refresh', () => workbench.refreshInstances())}
  />
{/snippet}

<div class="flex h-dvh w-full min-w-0 overflow-hidden">
  <aside class="hidden w-[272px] shrink-0 border-r border-border lg:block">
    {@render navigation()}
  </aside>
  <div class="flex min-w-0 flex-1 flex-col">
    <header
      class="flex min-h-16 shrink-0 items-center gap-2 border-b border-border bg-background/95 px-3 sm:gap-3 sm:px-6"
      style="padding-top: env(safe-area-inset-top)"
    >
      <Button
        variant="ghost"
        size="icon"
        class="icon-button lg:hidden"
        aria-label="Open navigation"
        onclick={() => (menuOpen = true)}><Menu class="size-5" /></Button
      >
      <div class="min-w-0 flex-1">
        <div class="flex min-w-0 items-center gap-2 text-sm">
          <FolderOpen class="hidden size-4 shrink-0 text-muted-foreground sm:block" /><span
            class="truncate font-medium"
            title={workbench.instance?.Workspace}
            >{workbench.instance ? workspaceName(workbench.instance.Workspace) : 'Workspace'}</span
          ><ChevronRight class="size-3 shrink-0 text-muted-foreground/50" /><span
            class="truncate text-xs text-muted-foreground"
            >{workbench.session ? `Session ${shortID(workbench.session.ID)}` : 'Overview'}</span
          >
        </div>
        {#if workbench.session}<p
            class="mt-0.5 truncate font-mono text-[10px] text-muted-foreground"
            title={workbench.session.Model}
          >
            {workbench.session.Model}
          </p>{/if}
      </div>
      <div class="flex shrink-0 items-center gap-1">
        <Button
          variant="ghost"
          class="icon-button gap-2 px-2 text-xs sm:px-3"
          onclick={() => openSettings()}
          title={connectionLabel}
          aria-label="Open settings"
          ><span
            class="hidden size-1.5 rounded-full sm:block {workbench.connection === 'connected'
              ? 'bg-primary'
              : 'bg-muted-foreground'}"
          ></span><span class="hidden sm:inline">{connectionLabel}</span><Settings2
            class="size-4"
          /></Button
        >
        <Button
          variant="ghost"
          size="icon"
          class="icon-button text-muted-foreground"
          disabled={!workbench.session}
          title="Session activity"
          aria-label="Session activity"
          onclick={() => (activityOpen = true)}><Radio class="size-4" /></Button
        ><Button
          variant="outline"
          class="icon-button px-3 text-xs"
          disabled={!workbench.instance || workbench.instance.Stopped}
          title="New session"
          onclick={() => (sessionOpen = true)}
          ><MessageSquarePlus class="size-4" /><span class="hidden sm:inline">New session</span
          ><span class="sr-only sm:hidden">New session</span></Button
        >
      </div>
    </header>
    <main class="flex min-h-0 min-w-0 flex-1 flex-col" id="main-content">
      {#if workbench.error}<div
          role="alert"
          class="flex shrink-0 items-start gap-2 border-b border-destructive/20 bg-destructive/5 px-4 py-3 text-xs leading-5 text-destructive sm:px-8"
        >
          <CircleAlert class="mt-0.5 size-4 shrink-0" /><span class="min-w-0 flex-1 break-words"
            >{workbench.error}</span
          ><Button
            variant="ghost"
            class="h-11 shrink-0 text-xs"
            onclick={() => openSettings('connection')}>Connection</Button
          >
        </div>{/if}
      {#if workbench.connection === 'connecting'}<div
          class="flex items-center justify-center gap-2 py-3 text-xs text-muted-foreground"
          role="status"
        >
          <LoaderCircle class="size-3.5 animate-spin" />Connecting to your harness…
        </div>{/if}
      <Conversation
        console={workbench}
        {busy}
        onConnection={() => openSettings('connection')}
        onWorkspace={() => (workspaceOpen = true)}
        onSession={() => (sessionOpen = true)}
        onResume={() => void runWorkbenchAction('resume', () => workbench.resumeInstance())}
        onCancelQueued={(identifier) =>
          void runWorkbenchAction(identifier, () => workbench.cancelQueuedMessage(identifier))}
        onDecision={resolvePermissionRequest}
        onPrompt={(text) => {
          if (sessionID) drafts[sessionID] = text;
        }}
      />
      <footer
        class="shrink-0 bg-background px-3 pt-2 sm:px-8"
        style="padding-bottom: max(.5rem, env(safe-area-inset-bottom))"
      >
        <div class="mx-auto max-w-4xl">
          <Composer
            bind:value={() => drafts[sessionID] ?? '', (value) => (drafts[sessionID] = value)}
            {disabled}
            {sending}
            running={workbench.status.running}
            cancelling={busy === 'cancel'}
            onSend={() => void sendDraftMessage()}
            onCancel={() => void runWorkbenchAction('cancel', () => workbench.cancelCurrentTurn())}
            onSettings={workbench.session ? openSessionSettings : undefined}
            settingsOpen={sessionSettingsOpen}
          />
          <div class="mt-1 flex items-center justify-between gap-2">
            <UsageBar statistics={workbench.statistics} /><span
              class="hidden shrink-0 text-[10px] text-muted-foreground/65 sm:block"
              >{workbench.session
                ? workbench.streamState === 'live'
                  ? 'Events connected'
                  : 'Syncing through the API'
                : 'mtt-harness · API client'}</span
            >
          </div>
        </div>
      </footer>
    </main>
  </div>
</div>

<Sheet.Root bind:open={menuOpen}
  ><Sheet.Content side="left" class="w-[min(88vw,310px)] gap-0 p-0" showCloseButton={true}
    ><Sheet.Header class="sr-only"
      ><Sheet.Title>Workspaces and sessions</Sheet.Title><Sheet.Description
        >Choose a workspace or session, or change the API connection.</Sheet.Description
      ></Sheet.Header
    >{@render navigation()}</Sheet.Content
  ></Sheet.Root
>
<SessionSettingsDialog
  bind:open={sessionSettingsOpen}
  models={workbench.models}
  model={sessionModel}
  modelID={sessionModel?.ID ?? workbench.session?.Model ?? ''}
  effort={workbench.session?.ReasoningEffort ?? ''}
  busy={busy === 'model' || busy === 'reasoning'}
  {disabled}
  error={sessionSettingsError}
  notice={sessionSettingsNotice}
  {pendingModel}
  onModelChange={(model) => void changeSessionModel(model)}
  onEffortChange={(effort) => void changeSessionEffort(effort)}
  onCancel={() => {
    pendingModel = null;
    sessionSettingsError = '';
  }}
  onConfirm={() => {
    if (pendingModel?.sessionID === sessionID) void changeSessionModel(pendingModel.model, true);
  }}
/>
<SettingsDialog
  bind:open={settingsOpen}
  bind:section={settingsSection}
  api={workbench.connection === 'connected' ? workbench.connectedAPIClient() : null}
  instance={workbench.instance}
  session={workbench.session}
  initialBase={preferences.base}
  initialToken={preferences.token}
  busy={busy === 'connect'}
  error={workbench.connection === 'error' ? workbench.error : ''}
  onConnect={connect}
  onProvidersChanged={() => workbench.refreshProviderCatalog()}
  onDisconnect={() => {
    workbench.disconnect();
    preferences = { base: preferences.base, token: '' };
    settingsOpen = false;
    toast.info('Disconnected');
  }}
/>
<WorkspaceDialog
  bind:open={workspaceOpen}
  models={workbench.catalog}
  catalogError={workbench.catalogError}
  error={workspaceError}
  busy={busy === 'workspace'}
  onCreate={createWorkspace}
/>
<SessionDialog
  bind:open={sessionOpen}
  models={workbench.models}
  defaultModel={workbench.instance?.DefaultModel ?? ''}
  busy={busy === 'session'}
  onCreate={createSession}
/>
<ActivityDialog bind:open={activityOpen} events={workbench.events} />
<Toaster theme="dark" closeButton position="top-right" offset="80px" mobileOffset="16px" />
