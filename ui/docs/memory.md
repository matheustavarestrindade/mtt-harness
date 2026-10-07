# Workspace Memory Display

## Panel

The Memory panel shows data for the selected workspace. A screen width of 1280 pixels or more uses a panel on the right side. A smaller screen uses a navigation drawer. The panel has a scrollbar. The panel does not change the conversation or message draft.

Use the header control to open or close the panel. Use the `Escape` key to close the navigation drawer. The browser keeps the setting in local storage. The setting does not contain credentials or memory text.

## API Data

The UI uses the harness API routes:

```text
GET   /instances/{id}/plugins/context/settings
PATCH /instances/{id}/plugins/context/settings
GET   /instances/{id}/plugins/context/statistics
```

Response keys have the Go format. Configuration keys have the plugin schema format. A `null` agent list becomes an empty array. A `null` object becomes an empty object. The client examines the workspace ID before display.

The panel uses an interval of 5 seconds for data requests. The interval is 1 second while a configuration change is in progress. A workspace change or connection change cancels previous requests. Data requests stop when the panel closes. Data requests also stop while the browser tab is not active.

A configuration request cancels previous data requests. A previous response must not replace the configuration result. An API error stays in the panel. It does not remove a message draft or stop the conversation. If previous usage data is available, the panel identifies the data as the last result.

## Usage Data

```text
Display                     Source
Memories                    memory/active + memory/consolidated
Retained history            memory/deleted + memory/superseded + memory/invalidated
Sources                     source/
Running jobs                job/running
Queued jobs                 job/pending
Prepared jobs               job/ready
Failed jobs                 job/failed
Direct saves                context.remember/saved_memories
Memory searches             context.main_search/queries
Checkpoints                 context.compactor/checkpoints
Messages reduced            context.compactor/removed_messages
Estimated tokens removed    context.compactor/estimated_tokens_removed
Embedding chunks            Sum of counters ending with /embedding_chunks
```

Memory versions do not increase the memory record number. The `Sources` number includes conversation messages and memory text. A `Prepared` memory job does not remove live context. The data includes the available history.

The API does not give a live context percentage or memory text through the routes. The panel does not make the values from different data.

## Agent Usage and Cost

The API gives usage by workspace agent and model. The panel shows model calls, input tokens, output tokens, cache data, reasoning tokens, and request time. The panel also shows `FailedCalls`. Workspace memory agents do not increase the selected session usage.

The model input total includes `Input`, `CacheRead`, and `CacheWrite`. The output total uses `Output`. The UI does not add `Reasoning` again.

The UI does not add costs from different currencies. A cost estimate has a label. The value `0` stays `0`. An empty usage record shows `0.0`. Model usage without a price shows `Unavailable`. Subscription models do not receive API token prices.

Costs include only model calls with price data from the API. The API does not identify the price status of a model call in a usage total. The `remember` tool uses embeddings but does not send a model request.

## Workspace Settings

Open `Memory settings` for the configuration form. The panel can set the plugin state to `ON` or `OFF`. It can also select the workspace worker model. The model list comes from the workspace API. The UI includes models with `Tools: false`.

A new worker model uses the model default reasoning effort. If the model ID stays the same, the reasoning effort does not change. The patch changes only supplied fields. Other workspace settings and harness defaults do not change.

The panel shows the applied plugin state. The value `Pending: true` identifies a change in progress. A worker model is necessary before memory can start. An API error keeps the configuration form available.

Use the API for other plugin parameters or harness settings. The UI does not add harness routes or runtime behavior.
