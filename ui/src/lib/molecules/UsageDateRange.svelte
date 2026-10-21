<script lang="ts">
  import { parseDate, type DateValue } from '@internationalized/date';
  import { CalendarDays } from 'lucide-svelte';
  import { MediaQuery } from 'svelte/reactivity';
  import type { DateRange } from '../atoms/usage';
  import { formatUsageDay, recentUsageRange, usageDays, utcDay } from '../atoms/usage';
  import { Button } from '../atoms/ui/button';
  import { Label } from '../atoms/ui/label';
  import * as Popover from '../atoms/ui/popover';
  import { RangeCalendar } from '../atoms/ui/range-calendar';
  let { value, onChange }: { value: DateRange; onChange: (value: DateRange) => void } = $props();
  const identifier = $props.id(),
    wide = new MediaQuery('(min-width: 700px)');
  let open = $state(false),
    draft = $state<{ start: DateValue | undefined; end: DateValue | undefined }>({
      start: undefined,
      end: undefined,
    });
  const selected = $derived(
    draft.start && draft.end ? { from: draft.start.toString(), to: draft.end.toString() } : null,
  );
  const valid = $derived(selected ? usageDays(selected).length > 0 : false);
  function setOpen(next: boolean) {
    if (next) draft = { start: parseDate(value.from), end: parseDate(value.to) };
    open = next;
  }
</script>

<div class="min-w-0 space-y-2">
  <Label for={identifier}>Date range · UTC</Label>
  <Popover.Root bind:open={() => open, setOpen}>
    <Popover.Trigger
      id={identifier}
      aria-label="Choose usage dates"
      class="flex h-11 w-full min-w-0 items-center justify-start gap-2 rounded-md border border-input bg-input/20 px-3 text-left text-sm"
      ><CalendarDays class="size-4 shrink-0" /><span class="truncate"
        >{formatUsageDay(value.from, true)} – {formatUsageDay(value.to, true)}</span
      ></Popover.Trigger
    >
    <Popover.Content
      align="start"
      class="w-auto max-w-[calc(100vw-1rem)] max-h-[var(--bits-popover-content-available-height)] overflow-y-auto p-0"
      collisionPadding={8}
      data-usage-date-range
    >
      {#if open}
        <div
          class="sticky top-0 z-10 flex flex-wrap gap-1 border-b border-border bg-popover p-2"
          aria-label="Usage date presets"
        >
          {#each [7, 30, 90] as days}<Button
              variant="ghost"
              class="h-11"
              onclick={() => {
                onChange(recentUsageRange(days));
                open = false;
              }}>Last {days} days</Button
            >{/each}
        </div>
        <RangeCalendar
          bind:value={draft}
          numberOfMonths={wide.current ? 2 : 1}
          maxValue={parseDate(utcDay())}
          calendarLabel="Usage dates"
        />
        <div
          class="sticky bottom-0 z-10 flex items-center justify-between gap-3 border-t border-border bg-popover p-3"
        >
          <span class="text-xs text-muted-foreground"
            >{selected && !valid ? 'Maximum 366 days.' : 'Inclusive UTC dates.'}</span
          ><Button
            class="h-11"
            disabled={!valid}
            onclick={() => {
              if (selected) {
                onChange(selected);
                open = false;
              }
            }}>Apply dates</Button
          >
        </div>
      {/if}
    </Popover.Content>
  </Popover.Root>
</div>
