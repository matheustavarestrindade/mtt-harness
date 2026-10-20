# UI Components

## Settings Pages

The settings pages use `SettingsPageTitle`. The component gives the same page title to the settings pages.

```text
Pages: General, Instructions, Sidekick, Watchers, Usage, Providers, Connection
Title: 18px, weight 500, line height 28px
Icon: 20px; 12px gap before the title
Description: 14px, line height 20px; 4px below the title
Description left edge: page content edge, 0px icon indent
Page inset: owned by SettingsDialog
```

The Connection settings page uses the same component. `Instructions` and Watchers use `PluginPanelFooter`.

## Component Map

```text
Usage graphs                    shadcn Chart + LayerChart
Date range                      shadcn RangeCalendar + Popover
Settings pages, Watcher sections -> Tabs
Thinking, tools, agents, task state, activity, usage -> Collapsible, Disclosure
Retrieval sources -> ToggleGroup
Agent task selection -> Toggle
CSV header selection -> Checkbox
Actions, forms -> Button, Input, Textarea
Tool selection, action menus -> DropdownMenu
Single/multiple selections -> Select
Model search -> Popover, Command
Settings, previews, navigation -> Dialog, Sheet
Field help, enabled state -> Tooltip, Switch
```

The control audit applies to `ui/src`. Only `atoms/ui` has `button`, `input`, `select`, and `textarea`. The UI uses HTML for message content.

## Charts

```text
Horizontal label space: min(130px, 25% of chart container width)
Category labels: LayerChart pixel-width truncation
Category ticks: explicit row keys, including a single category
Full labels and values: Chart.Tooltip and View data
```

## Tab Track

```text
Watcher tab button height: 44px
Watcher tab track height: 50px
Track padding: 3px on each edge
```

The tab highlight must stay in the tab track. The Settings tab track must not change the Watcher tab track.

## `open`

The `bits-ui` library keeps child components when the content is not shown. The UI uses the `open` value to remove tool output from a view.

The UI removes previous views with `URL.revokeObjectURL`, `Terminal.dispose`, and `WebSocket.close`.

```svelte
<Collapsible.Content>
  {#if open}
    <ToolOutput />
  {/if}
</Collapsible.Content>
```

Settings shows only the selected settings page. The file input uses `Input` with `class="hidden"`.

## Test Suite

```sh
npm run check
npm run test:e2e -- tests/settings-appearance.spec.ts
```

The test suite compares page titles, text dimensions, and tab highlights. The test suite includes keyboard access and a `320px` UI.
