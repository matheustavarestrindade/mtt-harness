<script lang="ts">
  import { FolderOpen, Layers, ChevronsUpDown, Plus, RefreshCw, Check } from 'lucide-svelte';
  import * as DropdownMenu from '../atoms/ui/dropdown-menu';
  import type { Instance } from '../atoms/types';
  import { workspaceName } from '../atoms/format';

  let {
    instances,
    instance,
    disabled = false,
    onSelect,
    onCreate,
    onRefresh,
  }: {
    instances: Instance[];
    instance: Instance | null;
    disabled?: boolean;
    onSelect: (instance: Instance) => void;
    onCreate: () => void;
    onRefresh: () => void;
  } = $props();
</script>

<DropdownMenu.Root>
  <DropdownMenu.Trigger
    class="flex h-12 min-w-0 flex-1 items-center justify-start gap-2 rounded-lg px-2 text-left outline-none hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
    aria-label="Switch workspace"
    title={instance ? `${instance.Workspace} — switch workspace` : 'Choose or create a workspace'}
    {disabled}
  >
    <span
      class="grid size-8 shrink-0 place-items-center rounded-lg bg-primary text-primary-foreground"
      >{#if instance}<FolderOpen class="size-4" />{:else}<Layers class="size-4" />{/if}</span
    >
    <span class="min-w-0 flex-1"
      ><span class="block truncate text-sm font-medium"
        >{instance ? workspaceName(instance.Workspace) : 'Select workspace'}</span
      ><span class="block truncate text-[11px] text-muted-foreground">Workspaces</span></span
    >
    <ChevronsUpDown class="size-3.5 shrink-0 text-muted-foreground" />
  </DropdownMenu.Trigger>
  <DropdownMenu.Content
    align="start"
    side="bottom"
    sideOffset={6}
    class="z-[60] min-w-56 max-w-[calc(100vw-1rem)]"
    aria-label="Workspace menu"
  >
    <DropdownMenu.Label class="text-xs text-muted-foreground">Workspaces</DropdownMenu.Label>
    <div class="max-h-64 overflow-y-auto overscroll-contain">
      {#each instances as workspace (workspace.ID)}
        <DropdownMenu.Item
          class="min-h-9 gap-2 pointer-coarse:min-h-11"
          data-workspace-id={workspace.ID}
          aria-current={instance?.ID === workspace.ID ? 'true' : undefined}
          title={workspace.Workspace}
          onSelect={() => onSelect(workspace)}
        >
          <FolderOpen class="size-4 shrink-0" />
          <span class="min-w-0 flex-1 truncate">{workspaceName(workspace.Workspace)}</span>
          {#if workspace.Stopped}<span class="text-[10px] text-muted-foreground">Stopped</span>{/if}
          {#if instance?.ID === workspace.ID}<Check class="size-3.5 shrink-0" />{/if}
        </DropdownMenu.Item>
      {:else}<p class="px-2 py-3 text-xs text-muted-foreground">No workspaces yet.</p>{/each}
    </div>
    <DropdownMenu.Separator />
    <DropdownMenu.Item class="min-h-9 pointer-coarse:min-h-11" onSelect={onCreate}
      ><Plus class="size-4" />New workspace</DropdownMenu.Item
    >
    <DropdownMenu.Item
      class="min-h-9 pointer-coarse:min-h-11"
      closeOnSelect={false}
      onSelect={onRefresh}><RefreshCw class="size-4" />Refresh workspaces</DropdownMenu.Item
    >
  </DropdownMenu.Content>
</DropdownMenu.Root>
