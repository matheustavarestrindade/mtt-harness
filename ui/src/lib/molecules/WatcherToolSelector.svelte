<script lang="ts">
  import { MediaQuery } from 'svelte/reactivity';
  import { Input } from '../atoms/ui/input';
  import { buttonVariants } from '../atoms/ui/button';
  import * as DropdownMenu from '../atoms/ui/dropdown-menu';
  import { Check, ChevronDown, Wrench } from 'lucide-svelte';
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
  let open = $state(false);
  const wideMenus = new MediaQuery('(min-width: 640px)');
  let searchInput = $state<HTMLInputElement | null>(null);
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

<div class="min-w-0 space-y-2">
  <DropdownMenu.Root bind:open>
    <DropdownMenu.Trigger
      {disabled}
      aria-label="Select watcher tools"
      class={buttonVariants({
        variant: 'outline',
        class: 'h-11 w-full min-w-0 justify-between px-3 font-normal',
      })}
    >
      <span class="flex min-w-0 items-center gap-2"
        ><Wrench class="size-4 shrink-0" /><span>Tools</span></span
      >
      <span class="ml-auto mr-2 text-muted-foreground">{grants.length} selected</span>
      <ChevronDown class="size-4 shrink-0 text-muted-foreground" />
    </DropdownMenu.Trigger>
    <DropdownMenu.Content
      aria-label="Watcher tools"
      align="start"
      sideOffset={6}
      collisionPadding={8}
      class="z-[60] max-h-[min(24rem,var(--bits-dropdown-menu-content-available-height))] w-[min(22rem,calc(100vw-2rem))] overflow-y-auto"
      onOpenAutoFocus={(event) => {
        event.preventDefault();
        searchInput?.focus();
      }}
    >
      <div class="sticky top-0 z-10 bg-popover p-2">
        <Input
          bind:ref={searchInput}
          aria-label="Search watcher tools"
          placeholder="Search tools…"
          bind:value={query}
          onkeydown={(event) => {
            if (!['ArrowDown', 'ArrowUp', 'Escape', 'Tab'].includes(event.key))
              event.stopPropagation();
          }}
        />
      </div>
      <DropdownMenu.Group>
        {#each unavailable as grant (grant.name)}
          <DropdownMenu.CheckboxItem
            class="min-h-11"
            checked
            aria-label={grant.name}
            onCheckedChange={() => onChange(grants.filter((value) => value.name !== grant.name))}
            ><span class="min-w-0 break-all">{grant.name} · Unavailable</span
            ></DropdownMenu.CheckboxItem
          >
        {/each}
        {#each filtered as tool (tool.name)}
          {@const selected = grants.find((grant) => grant.name === tool.name)}
          {#if tool.operations?.length}
            <DropdownMenu.Sub>
              <DropdownMenu.SubTrigger
                class="min-h-11 gap-2"
                aria-label={`${tool.name} operations`}
                title={tool.description}
              >
                <span class="flex size-4 shrink-0 items-center"
                  >{#if selected}<Check class="size-4" />{/if}</span
                >
                <span class="min-w-0 flex-1 break-all">{tool.name}</span>
                <span class="shrink-0 text-xs text-muted-foreground"
                  >{selected?.operations?.length ?? 0} selected</span
                >
              </DropdownMenu.SubTrigger>
              <DropdownMenu.SubContent
                aria-label={`${tool.name} operations`}
                side={wideMenus.current ? 'right' : 'bottom'}
                sideOffset={4}
                avoidCollisions
                collisionPadding={8}
                class="z-[70] max-h-[min(24rem,var(--bits-dropdown-menu-content-available-height))] w-56 max-w-[calc(100vw-1rem)] overflow-y-auto"
              >
                <DropdownMenu.CheckboxItem
                  class="min-h-11"
                  checked={!!selected}
                  aria-label={`Enable ${tool.name}`}
                  onCheckedChange={(checked) => toggleTool(tool, checked)}
                  >Tool enabled</DropdownMenu.CheckboxItem
                >
                <DropdownMenu.Separator />
                {#each tool.operations as operation}
                  <DropdownMenu.CheckboxItem
                    class="min-h-11"
                    checked={selected?.operations?.includes(operation) ?? false}
                    disabled={!selected}
                    onCheckedChange={(checked) => toggleOperation(tool, operation, checked)}
                    >{operation}</DropdownMenu.CheckboxItem
                  >
                {/each}
              </DropdownMenu.SubContent>
            </DropdownMenu.Sub>
          {:else}
            <DropdownMenu.CheckboxItem
              class="min-h-11"
              checked={!!selected}
              onCheckedChange={(checked) => toggleTool(tool, checked)}
              title={tool.description}
            >
              <span class="min-w-0 break-all">{tool.name}</span>
            </DropdownMenu.CheckboxItem>
          {/if}
        {/each}
        {#if !filtered.length && !unavailable.length}<DropdownMenu.Item disabled class="min-h-11"
            >No matching tools</DropdownMenu.Item
          >{/if}
      </DropdownMenu.Group>
    </DropdownMenu.Content>
  </DropdownMenu.Root>
  <p class="text-xs leading-relaxed text-muted-foreground">
    Selected tools run without permission prompts. Continue, interfere, and discovery are always
    available.
  </p>
</div>
