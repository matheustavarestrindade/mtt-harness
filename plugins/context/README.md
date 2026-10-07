# Context Plugin

**Status: implemented.** The UI control is subsequent work. The harness attaches the plugin. The default plugin state is `OFF`.

See `TODO.md` for the work sequence.

The project rules are in `AGENTS.md` and `Spec.md`.

The UI rules are in `ui/AGENTS.md`.

## Purpose and Boundary

The plugin keeps memory for one workspace across sessions. A new session receives the workspace memory snapshot in the initial model request. A workspace must not receive memory from a different workspace.

The plugin contains memory tools, memory workers, storage, memory search, and context compaction. Use harness interfaces for runtime access. Imports must not refer to harness internal packages. The harness gives general interfaces for plugin settings, model requests, context limits, and start/stop control.

## Tools

Tool discovery supplies full definitions before a tool call. Tool results give operation data or an error cause, not system instructions.

The tools are:

- The `ctx_drop` tool selects conversation message IDs from the context view. It accepts `remember` and optional categories. The messages stay in the context view until context compaction.

- The `ctx_wrapup` tool applies context compaction before the next model request.

- The `remember` tool adds or corrects workspace memory through a smaller model. The input has text and optional categories.

- The `search_memory` tool gives memory records or source messages. A query can have text, a category, a compression level, and `include_deleted`.

- The `list_memory_categories` tool gives available category data.

Stable IDs are necessary for conversation message selection. Memory IDs and categories stay in the database, not the memory snapshot text. The AI uses a query to get more information.

## Memory Sources and Storage

```text
ctx_drop / remember
        |
        v
Source messages + durable memory jobs
        |
        v
Smaller model -> source validation -> L / M / H text
        |
        v
Embeddings + memory records ready for context compaction
        |
        v
ctx_wrapup / 80% intervention -> new context view
```

The plugin starts a memory job when it accepts `ctx_drop`. It does not wait for 65%. With `remember: false`, the memory archive keeps the source messages without a memory summary. With `remember: true`, source messages, memory summaries, and embeddings must be in the database before context removal.

Postgres keeps plugin data and memory jobs in the `context_plugin` schema. The plugin uses pgvector for memory search. The runtime and test images include pgvector 0.8.1. A model makes memory summaries. The plugin makes vectors from text chunks below the tokenizer limit.

A memory source includes message IDs, roles, user approval, and sequence data. Text from a user message is applicable. A model decision that the user accepts is also applicable. An illustrative example or model proposal must not become a user requirement without source data.

The `remember` worker uses memory search before a change. Keep previous memory versions. A correction must also change a memory idea that uses the previous fact. The operation must not change a different memory topic.

The memory archive keeps source messages after context compaction, the `revert` operation, or session deletion. Memory search after session deletion is available with `include_deleted: true`. The tool identifies history data. Memory search must not make a session available after session deletion.

## Context Limits

Calculate the input budget for the selected model:

```text
input_budget = model_context_limit - output_token_allowance
```

Include system text, tool schemas, image data, audio data, task state, and memory in the token count. Use provider token counts when available. Identify a token estimate when necessary.

The input thresholds are:

- At 50%, add a context notice. It tells the AI to examine context and use `ctx_drop`.
- At 65%, add a context notice to use `ctx_drop` and `ctx_wrapup` immediately.
- At 80%, hold the next model request and apply context compaction without model approval.

Keep system instructions, the last user request, active tool work, and `TODO` / `DOING` data. A tool call and the tool result must stay together. Write memory records before source messages are removed from the context view. The plugin must change the context view in one transaction after the active tool group.

After context compaction at 80%, the input budget must be below 50%. The plugin gives an error when it cannot keep necessary content in the input budget. A memory job with an error must not cause content removal. The context fitter must use plugin policy when the plugin state is `ON`.

## Memory Snapshots and Cache

After the initial session snapshot, change the memory snapshot only when context compaction removes content. The text, sequence, and position must not change between context refreshes. A memory worker can change the database, but the memory snapshot must wait. A memory search can give new information through tool results.

The next memory snapshot replaces the previous snapshot. Do not put previous memory snapshots in message history. The text has a compression level and memory text, without memory IDs or categories.

```text
<memory>
L Detailed memory from the context that was just reduced.
M The main details from a previous memory snapshot.
H The essential fact from an older memory snapshot.
I A broader memory idea from the historian.
</memory>
```

New information uses `L`. At the next context refresh, previous `L` becomes `M`, and previous `M` becomes `H`. The `H` value does not change. Only the historian makes `I` memory records. A `ctx_wrapup` operation without context removal does not change compression levels.

Record the memory version and compression level given to the session. A query or correction can make `L` necessary at the next context refresh. The plugin selects text from the database without a new model call. The 80% limit also applies to the memory snapshot.

Context notices must not change the prompt prefix. Examine task state, task reminders, tool definitions, and provider request format for the same cache requirement. Make sure the request sent to the provider keeps the prompt prefix.

## Historian

The historian is a workspace agent. Memory search usage and plugin settings identify memory records for memory consolidation. The historian makes a memory idea from memory records on the same memory topic. Keep source references to the initial memory records. Memory search can give the source data when necessary.

Use memory records and memory sources to make the memory idea. Keep user requirements, limits, and rule exceptions. The plugin must record worker memory search usage independently from model memory search usage. A historian memory record changes a session memory snapshot only at a context refresh.

## Settings and UI Control

The database keeps plugin settings for the harness and for a workspace. The plugin uses a workspace value before the harness value. Select worker models from the model list. Settings include the historian period, input thresholds, memory snapshot limit, worker count, retry limit, and cost limit.

The UI ON/OFF control is subsequent work. UI code, dependencies, the test suite, and documents stay in `ui/`. The client reads plugin state and settings through the API. It shows if the value comes from the workspace or the harness.

- With plugin state `ON`, examine the settings and start workspace workers. A new session receives an initial memory snapshot. A previous session keeps the context refresh rule.
- With plugin state `OFF`, stop new memory jobs and context refreshes for the workspace. After the active tool group, the plugin must complete or cancel accepted work. Keep memory records, the memory archive, and the context view given to the model.
- An ON/OFF request must not remove data, expand the full memory archive into context, or stop a tool group. A workspace control must not change a different workspace.

The UI must show when a change waits for active work. It must not show `ON` or `OFF` before the harness applies the change. See section 7 in `TODO.md`.

## Usage Data

The plugin must record model usage by workspace, agent name, run ID, provider, and model. Agent roles include the context compactor, memory writer, and historian. Keep token counts, cache data, cost, duration, operation status, and retry data. Workspace agent cost must not increase session cost. The session ID identifies the memory source only.

Keep memory search usage, compression levels, and a usage counter for memory snapshots, memory correction, and memory consolidation. The plugin records local inference metrics. The local model does not have an API cost. The API gives usage data. The UI display is subsequent work.

The next harness process must continue memory jobs. Do not write the same memory record again. The plugin must record the cost of a model request one time.

A previous worker result must not replace new context after the `revert` operation, session deletion, or a plugin state change. Stop workers before you close the database connection. Keep database content when the plugin is removed.

## Configuration

The plugin uses Postgres and the optional MiniLM adapter. The standard Docker image includes the adapter and model assets. Embeddings are not available in the `core` image. The model must have a context limit. An available worker model is necessary before memory work can start.

Use the plugin API to change settings. The database key is `plugin.context`. A workspace object can override a harness field. A JSON `null` removes the workspace field. The API gives the plugin state and configuration.

```text
GET   /plugins
GET   /plugins/context/settings
PATCH /plugins/context/settings
GET   /instances/{id}/plugins/context/settings
PATCH /instances/{id}/plugins/context/settings
GET   /instances/{id}/plugins/context/statistics
GET   /instances/{id}/agent-statistics
```

For example, use an available model from `GET /instances/{id}/models`:

```json
{"enabled":true,"worker_model":"provider/model"}
```

An empty `historian_model` uses `worker_model`. Empty reasoning effort uses the model default. The schema in the settings response gives parameter descriptions, defaults, and units. The configuration rules for provider URLs, credentials, and prices do not change.

The default input thresholds are 50%, 65%, and 80%. The input budget after context compaction is below 45%. The memory snapshot limit is 1000 estimated tokens. A workspace can run 2 memory jobs. The process has a limit of 32 active memory jobs. Model requests run one at a time in a workspace.

The default retry limit is 2. A memory job attempt has a 120000 ms time limit. The database keeps completed text chunks and memory extraction pages. After cancellation, a worker does not start a subsequent text chunk. MiniLM completes the text chunk in progress before it stops.

The default cost limit is 0. A value of 0 does not stop work because of cost. With a value above 0, workspace usage and a request estimate control subsequent requests. The currency must agree with the model prices. A cost estimate is not available for unknown prices. Subscription models do not receive API token prices.

## Database

The `documents` table keeps memory records, memory versions, source messages, context views, memory jobs, and worker output. Previous memory versions do not change. The `vectors` table keeps text chunks and the model identity. The `metrics` table keeps operation counters. A workspace ID is necessary for the tables.

Memory search selects the workspace and category data before it compares vectors and literal text. The tool output has a 16 KiB text limit. A cursor continues long source text without a change to UTF-8 bytes.

Workers use worker leases and source epochs. The plugin writes source messages to the memory archive before the core history operation. A subsequent process examines the history operation against the conversation store. It does not make a session available after session deletion.

The model receives stable message labels. Source text is data, not system instructions. The memory archive does not include internal provider continuation data. The primary request keeps necessary provider data with the tool group.

## Removal

Set `enabled` to `false` through the API. Wait until `Pending` is `false`. This stops new work and keeps the context view given to the model.

To remove the plugin, remove `attachContextPlugin` from the program.

Remove the files:

```text
cmd/mtt/context_plugin.go
cmd/mtt/plugin_embeddings.go
```

Remove `newContextEmbeddings` from the files:

```text
cmd/mtt/tool_search_semantic.go
cmd/mtt/tool_search_core.go
```

Then remove `plugins/context/`.

Keep the general harness interfaces if a different plugin uses them. Keep the `context_plugin` schema for memory records and source data.

The UI control is in section 7 of `TODO.md`.
