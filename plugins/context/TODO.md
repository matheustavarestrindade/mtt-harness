# Context Plugin Work Plan

The code and test suite are available. The UI control is subsequent work. Use `[x]` only when the code agrees with the requirements and the test suite. The behavior is in `README.md`.

## 1. Harness Interfaces

- [x] Add interfaces for plugin settings, storage, model requests, model usage, and start/stop control.
- [x] Give the plugin the input budget from the last model selection. Add a context fitter interface for plugin policy.
- [x] Give conversation messages stable IDs and roles. Do not change source message text.
- [x] Apply a context view change only after the active tool group.
- [x] Add workspace control and worker cancellation. Make sure plugin imports agree with project rules.
- [x] Examine the prompt prefix for data from task state, task reminders, tool discovery, and context notices.

## 2. Storage and Settings

- [x] Add tables for memory records, versions, categories, source messages, source references, memory jobs, context views, and usage data.
- [x] Add pgvector to the runtime and test database images.
- [x] Use workspace IDs for memory data. Give sessions in the same workspace access to the memory records.
- [x] Add plugin settings at harness and workspace scope. Include `ON/OFF`, model IDs, input thresholds, historian policy, worker count, retry limit, and cost limit.
- [x] Make sure worker models agree with the model list and workspace model policy.
- [x] Write source messages to the memory archive before live conversation content is removed. Keep session deletion data for history queries.
- [x] Keep memory jobs in the database. Add worker leases, cancellation, and retry limits. Write the result of a memory job only one time.

## 3. Tools and Memory Sources

- [x] Add `ctx_drop`. Keep source messages until context compaction. Add `remember` and category input fields.
- [x] The `remember` tool must write input text and embeddings before it gives a result. Do not change the text. A model request or memory job is not necessary. Use `old_text` for a correction by literal text.
- [x] Give workers message roles, source IDs, and user approval data. Do not change an illustrative example or model proposal into a user requirement.
- [x] The plugin must examine worker output and write L/M/H text and memory sources in one transaction.
- [x] Use memory search to find a previous memory record that does not agree with new source data. Keep previous versions and correct memory ideas that use the previous fact.
- [x] Make embeddings for memory summaries and source messages. The plugin must divide text with the tokenizer. Do not discard text chunks.
- [x] Add `search_memory` with text, category, compression level, source messages, and history queries. Include `include_deleted`.
- [x] Add `list_memory_categories`. Keep memory IDs and categories in the database, not the memory snapshot text.
- [x] Give memory search only the applicable workspace and category data. The tool uses vectors and compares literal text. Limit tool output.
- [x] Add tool descriptions, input schemas, and search documents. Make tool discovery necessary before a tool call. Tool results must give data, not system instructions.

## 4. Context Compaction and Cache

- [x] Add context notices at 50% and 65% after the prompt prefix.
- [x] Add `ctx_wrapup`. Use one transaction for the context view and memory records.
- [x] Hold the next model request at 80%. Apply context compaction without model approval. Decrease the input budget below 50%.
- [x] Keep system instructions, the last user request, active work, task state, tool groups, and necessary provider data.
- [x] Keep source messages when a memory job gives an error. Give an error if the plugin cannot keep necessary content in the input budget.
- [x] Give new sessions the initial workspace memory snapshot.
- [x] Keep the same memory snapshot between context refreshes. Replace it only with context removal. Do not put memory snapshots in message history.
- [x] Keep compression level data by session and memory version. Use new `L`, then `M`, then `H`. Only historian memory records use `I`.
- [x] Do not change compression levels if `ctx_wrapup` does not remove context.
- [x] Give new data through memory search tool results. Do not change the memory snapshot until a context refresh. Apply a change back to `L` at the context refresh.
- [x] Keep memory snapshot text in the input budget. Keep text sequence stable.

## 5. Historian

- [x] The plugin must record model memory search usage and worker memory search usage independently.
- [x] Use the historian period and usage data to select memory records on the same memory topic.
- [x] Make `I` memory records from source data. Keep source references to the initial memory records.
- [x] Put memory ideas in the active view. Do not remove the source data from memory search.
- [x] Keep user requirements, limits, and rule exceptions. Mark previous memory versions as history after a correction.
- [x] Run context compaction at 80% before historian maintenance.

## 6. Usage and Session Control

- [x] Add usage records with workspace, agent name, run ID, and model request ID. Include a model request with an error and retry data.
- [x] Keep provider cost, cost estimates, cache data, unknown prices, and subscription billing rules.
- [x] Do not add workspace agent cost to session cost. The plugin must record a model request only one time in workspace cost.
- [x] Add usage counters for memory search, context compaction, historian work, and embeddings.
- [x] Reject a worker result from a previous context after the `revert` operation, session deletion, cancellation, or a plugin state change.
- [x] Keep the memory archive available after session deletion. Do not make the live session available again.
- [x] Stop workers before you close the database connection. Keep database content after plugin removal.

## 7. UI ON/OFF Control

- [x] Add an API for available plugins, plugin state, plugin settings, and ON/OFF requests.
- [x] Apply ON/OFF after the active tool group. Show when a change waits for active work.
- [ ] Add a control in `ui/` for the harness value and workspace value.
- [ ] Show the source of the plugin setting, the plugin state, and an operation error. Wait for the change before the UI shows the new plugin state.
- [x] Stop new memory work at `OFF`. Keep the database and context view given to the model. Do not expand the memory archive or change the prompt prefix.
- [x] The plugin must examine settings before workers start at `ON`. A session in progress changes the memory snapshot only at a context refresh.
- [ ] Add a test suite with `Tab`, `Enter`, a small screen, a connection error, and different workspace settings.

## 8. Test Suite and Documents

- [x] Make sure provider requests keep the same memory snapshot between context refreshes.
- [x] Add a test suite for L/M/H/I sequence, new session memory, category queries, source retrieval, and history queries.
- [x] Add a test suite for model decisions that the user accepted. Reject a memory record without the necessary source and user approval data.
- [x] Add a test suite for context notices and context compaction at 80% with large tool output. Use different selected model limits.
- [x] Add a test suite for a memory job with an error, bad JSON, a database error, process start/stop, retry data, and cancellation.
- [x] Make sure context selection keeps tool calls with tool results. Do not give internal provider data to memory workers.
- [x] Add a test suite for workspace boundaries and memory search after session deletion.
- [x] Add a test suite for ON/OFF during active requests and memory jobs. Make sure historian work and source retrieval agree with the requirements.
- [x] The test suite must measure usage counters, cost, context tokens, cache usage, and worker duration for workspace agents.
- [x] Run the Go, Postgres, and architecture test suites. Use `go test -race` and `scripts/ste` for the applicable files.
- [x] The documents must agree with the code. Include configuration and removal steps in `Spec.md`, `routes.md`, and the plugin documents.
- [ ] Run the UI test suite and change UI documents with the UI control in section 7.

Worker system instructions identify illustrative examples and model proposals. The plugin examines roles, source text, and user approval data. Use the workspace worker model to examine memory extraction quality.
