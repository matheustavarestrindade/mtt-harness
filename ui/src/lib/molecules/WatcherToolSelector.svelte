<script lang="ts">
  import { Input } from '../atoms/ui/input';
  import type { WatcherTool, ToolGrant } from '../atoms/watcher';
  let {
    tools,
    grants,
    onChange,
    disabled = false,
  }: {
    tools: WatcherTool[];
    grants: ToolGrant[];
    onChange: (value: ToolGrant[]) => void;
    disabled?: boolean;
  } = $props();
  let query = $state('');
  const filtered = $derived(
    tools.filter((tool) =>
      `${tool.name} ${tool.categories?.join(' ')}`
        .toLowerCase()
        .includes(query.trim().toLowerCase()),
    ),
  );
  const unavailable = $derived(
    grants.filter((grant) => !tools.some((tool) => tool.name === grant.name)),
  );
  function toggleTool(tool: WatcherTool, enabled: boolean) {
    const next = grants.filter((grant) => grant.name !== tool.name);
    if (enabled)
      next.push({
        name: tool.name,
        ...(tool.operations?.length
          ? {
              operations: tool.operations.filter((operation) =>
                ['read', 'list', 'glob'].includes(operation),
              ),
            }
          : {}),
      });
    onChange(next);
  }
  function toggleOperation(tool: WatcherTool, operation: string, enabled: boolean) {
    onChange(
      grants.map((grant) =>
        grant.name !== tool.name
          ? grant
          : {
              ...grant,
              operations: enabled
                ? [...new Set([...(grant.operations ?? []), operation])]
                : (grant.operations ?? []).filter((value) => value !== operation),
            },
      ),
    );
  }
</script>

<fieldset class="min-w-0 space-y-3" {disabled}>
  <legend class="mb-2 text-sm font-medium">Tool access</legend>
  <p class="text-xs text-muted-foreground">
    Enabled tools run without permission prompts. Discovery shows only this selection. Continue,
    interfere, and tool discovery are always available.
  </p>
  <Input aria-label="Search watcher tools" placeholder="Search tools…" bind:value={query} />
  <div class="max-h-72 space-y-1 overflow-y-auto rounded-md border border-border p-2">
    {#each unavailable as grant (grant.name)}<label
        class="flex min-h-11 items-center gap-3 px-2 text-xs"
        ><input
          type="checkbox"
          class="size-4 accent-foreground"
          checked
          onchange={() => onChange(grants.filter((value) => value.name !== grant.name))}
        /><span class="break-all">{grant.name} · Unavailable</span></label
      >{/each}
    {#each filtered as tool (tool.name)}
      {@const selected = grants.find((grant) => grant.name === tool.name)}
      <div class="min-w-0 rounded-sm px-2">
        <label
          class="flex min-h-11 cursor-pointer items-center gap-3 text-sm"
          title={tool.description}
        >
          <input
            type="checkbox"
            class="size-4 accent-foreground"
            checked={!!selected}
            onchange={(event) => toggleTool(tool, event.currentTarget.checked)}
          />
          <span class="min-w-0 break-all font-mono text-xs">{tool.name}</span>
        </label>
        {#if selected && tool.operations?.length}<div
            class="mb-2 grid grid-cols-2 gap-x-2 border-l border-border pl-5 sm:grid-cols-3"
          >
            {#each tool.operations as operation}<label
                class="flex min-h-11 items-center gap-2 text-xs"
                ><input
                  type="checkbox"
                  class="size-4 accent-foreground"
                  checked={selected.operations?.includes(operation) ?? false}
                  onchange={(event) =>
                    toggleOperation(tool, operation, event.currentTarget.checked)}
                />{operation}</label
              >{/each}
          </div>{/if}
      </div>
    {/each}
    {#if !filtered.length}<p class="p-3 text-xs text-muted-foreground">No matching tools.</p>{/if}
  </div>
</fieldset>
