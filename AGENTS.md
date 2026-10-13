# mtt-harness — Agent Instructions

## Decisions

- Language: Go.
- Plugins: Go packages in the module. External tools come from MCP servers (`mcp.json`, stdio or HTTP).
- First user surface: a headless API (HTTP + WebSocket).
- Database: Postgres. The optional context plugin provides workspace memory and vector recall.
- Application configuration: `mtt.json` is the bootstrap file (port, database URL, file paths, optional token). The database holds the settings and the provider secrets.
- Benchmark configuration: Docker Compose reads the local, gitignored `.env` file and forwards `MTT_READ_LINE_NUMBERS` into the harness. `.env.example` documents the switch. The Go process reads the environment once at tool registration; it does not load `.env` itself.

## Writing Style

Product documents (`Spec.md`, and later the files in `docs/`) use ASD-STE100 Simplified Technical English.

The default model prompt applies ISO 24495-1 plain-language principles to user-facing communication, together with ASD-STE100 for English. Replies must be relevant, findable, understandable, and usable for the user's task. Match the user's language, preserve technical identifiers, and keep compact agent notes distinct from user-facing replies.

Check a product document with:

```sh
./scripts/ste analyze FILE --project-dictionary ste/terms.json --spacy
```

Rules:

- A `PASS` result is necessary before a commit of a product document.
- Add a new technical noun or technical verb to `ste/terms.json`.
- The file `scripts/ste` sets the library path for the checker on NixOS.

## Source of Truth

`Spec.md` gives the design: one loop, four boundaries, stages, plugins, tools, processes, the database, the API, the atoms/molecules/organisms layers, and the milestones.

## Web UI Boundary

- `ui/` is a standalone API client, not part of the harness runtime. Its package, dependencies, configuration, build, tests, proxy, and documentation belong inside `ui/`.
- Do not change the harness, API behavior/contracts, Go code, database, runtime configuration, or root Docker services to satisfy a UI requirement unless the user explicitly authorizes that backend change. Adapt the client to the existing API and document gaps instead.
- Root `routes.md` documents the implemented public API. Check handlers and atom JSON shapes before changing client types or route documentation; the actual wire format is authoritative.
- The UI uses Svelte 5 runes, Tailwind CSS, reusable shadcn-svelte components, lucide-svelte icons, and Sonner notifications. It must support desktop and mobile, including keyboard access, touch targets, small screens, and long message/tool output.
- The UI follows the atoms → molecules → organisms dependency layers. Put primitives/types in `ui/src/lib/atoms`, composed controls and clients in `molecules`, and state/workflow/layout owners in `organisms`. Enforce the direction with the UI layer check.
- UI-specific design and setup documents live under `ui/`. See `ui/AGENTS.md` before editing the client.

## Go Readability Decisions

- Use complete, descriptive variable, parameter, field, and receiver names. Use `harnessRuntime`, `toolRegistry`, `configuration`, `operationContext`, `operationError`, `request`, and `responseWriter`; do not use `h`, `reg`, `cfg`, `ctx`, `err`, `r`, or `w`. Established domain acronyms such as ID, URL, HTTP, JSON, SQL, and MCP are acceptable.
- Function and method names must identify the operation and its target, such as `requestOAuthTokens`, `activateDiscoveredTools`, and `pollDeviceAuthorization`. Audit surrounding implementation and call sites, not just the example named in a review. Keep standard interface contracts such as `Close`, `Read`, and `Run` where required.
- Prefer guard clauses and early returns. Handle invalid input and failed operations first; keep the successful path at the outer indentation level. Avoid `else` after a branch that returns.
- Use named error helpers for repeated handling: startup requirements, test assertions, HTTP error responses, and contextual error wrapping. Runtime failures must return errors to their caller. A failed tool, provider, or database operation must not terminate the harness. Do not swallow errors or recover arbitrary programming panics.
- Keep `cmd/mtt/main.go` as a short entry point. Put argument parsing, application wiring, authentication, storage setup, provider loading, and MCP loading in separate, clearly named files.
- Give each file one responsibility. Split files by behavior before adding more unrelated functions; a line-count target must not create meaningless wrapper packages.
- Prefer one authoritative registry or service per capability. Do not add parallel registries or placeholder layers that are disconnected from the runtime.
- Comments explain ownership, ordering, cancellation, and surprising decisions. Do not repeat the code in comments. Exported contracts and concurrency invariants need human-readable documentation.
- Keep refactors separate from functional changes and verify behavior with fresh tests. Passing tests and the STE checker do not establish specification completeness.

## Model-Facing Tool Contracts

- Treat tool descriptions and JSON Schemas as instructions the model must be able to use without reading implementation code. Describe every input property's purpose, units, defaults, valid choices, and special values such as zero or an empty string where applicable.
- State material behavior: path resolution, file replacement, output/retention limits, harness IDs versus OS PIDs, blocking/background behavior, and what cancellation actually stops.
- Harness-generated tool feedback is factual data, not instructions. Report the failed operation, its cause, and available observed metadata without corrective advice or repeated error wrappers. The model chooses the next action. Continuation positions are data; instructions for using them belong in the tool definition. Preserve literal file/command/MCP output rather than rewriting it as harness advice.
- The startup prompt describes current discovery and tool-priority policy. Keep tool-specific operation/parameter instructions in the complete registry-provided description and schema, not a second hand-maintained walkthrough. Do not include retired-tool migration notes in a new session's startup prompt. Consult loaded definitions; discover missing definitions before use.
- Preserve descriptions and defaults when adding dynamic schema metadata such as the agent model list. Verify the definition sent in provider requests, not only the static tool declaration.
- Built-in `bash.timeout` and `bash.interval` are milliseconds. A missing or zero timeout means no automatic process timeout; `wait` defaults to true. Foreground calls set notification mode `none` before process start and return output only as tool results. `notify` applies to `wait:false`, defaults to exit, accepts none/exit/error/interval, and interval notifications need a positive interval.
- MCP descriptions and schemas belong to their servers. Preserve that metadata; do not invent units or defaults for unknown external parameters.

## Tool Discovery

- Tool discovery embeds full tool descriptions, usage examples, and input schemas. Built-in index-only usage documents are in `internal/tools/search_documents.go`; `harness.ToolSearchDocumentation` is optional for plugins. Do not add keyword/alias lists such as `SearchTerms`.
- `search_tool` returns compact `Name`/`Categories` references. The loop resolves full callable definitions from the authoritative registry for the next model request. Search documents and vectors must not be sent in provider requests or tool results.
- `search_tool` is the initial discovery capability. `file_actions` is registered but is loaded only after discovery; child sessions additionally have their lifecycle `finish` tool. The default startup prompt contains the compact catalog, not embedded full tool definitions. Resolve discovered groups from the current registry, reuse loaded schemas, and preserve historical calls/results. Unregistered names return factual errors instead of aliases or replacement suggestions.
- `providers.json.tool_search` selects `auto`, `semantic`, or `lexical` with separate cosine thresholds. `auto` uses MiniLM when available and permanently falls back to TF-IDF after a reported initialization/inference failure. Cancellation never starts fallback work. Strict `semantic` mode returns recoverable tool errors on inference failure.
- MiniLM uses the optional Hugot v0.7.0 pure-Go adapter in `internal/molecule/embedding/minilm`, behind the `semantic` build tag. The only application import is in `cmd/mtt/tool_search_semantic.go`. The core build and registry depend on `toolsearch.Searcher`, not Hugot. Keep model-specific code and dependencies inside that adapter so it can be removed.
- Tool discovery uses the pinned official `all-MiniLM-L6-v2` model with the pure-Go adapter. Standard Docker builds also include text-only EmbeddingGemma 2 for workspace memory, with native ONNX Runtime and Hugging Face Tokenizers behind `gemma` + CGO. The standard application image uses Debian Bookworm; the `core` target stays CGO-free Alpine and skips model assets. Both models run in-process; downloads occur at image build time only.
- MiniLM inputs have a 256-WordPiece limit. Split full documents with the real tokenizer, retain every chunk, and rank the best chunk per tool. The cache invalidates changed/removed documents. TF-IDF uses the same complete metadata, weighting capability descriptions above invocation examples. Workspace memory and archived conversation retrieval belong to the optional context plugin.
- Exact tool names and category-only discovery bypass vector work. A failed strict search must return a recoverable tool error, rather than an empty successful result. Startup logs show the requested/selected backend and automatic fallback causes.

## File Editing and Read Benchmark

- `file_actions` accepts either `path` or 1–32 `paths`, with one shared array of 1–32 ordered actions and one return selection. Each target sees its own staged content. Resolve/check every target, reject duplicate resolved paths and overlapping mutation targets, and acquire gates in canonical order. Prepare all targets, previews and replacement temporary files before any commit. Preparation failure changes no destinations. Commits run in input order and are atomic per target, not across files. A commit I/O failure stops later targets and reports failed/committed target indexes; never claim batch atomicity or retry committed targets automatically. External editors do not join the gates.
- Edits require explicit `return`: summary, diff (context_lines default 3, range 0–100), or read. No implicit diff. With return, suppress intermediate read/list output. Single-target output has no banners, logs, counts or Return labels. Multi-target read/list output uses short quoted-path labels, diffs identify paths themselves, and summaries combine status counts. One 200-line/16-KiB budget spans all targets and labels; omitted targets have a 1-based next target index. There is no `on_error`. Failure output contains only facts and commit metadata where applicable; formatting performs no I/O and preserves Go causes.
- File-action guidance must not require a fresh read before every edit. Reuse relevant content from instructions, conversation, prior reads, previews, and reported facts. Fully specified append/prepend operations, intended whole-file writes, and known exact replacements can run directly with the chosen return. After failure, the model decides whether any further inspection is needed. Pre-read only when missing structure, formatting, target identity, current line positions, or evidence of intervening edits affects the operation. Keep context-based patch anchors and range edits grounded in current information. Inspect returned output once, then run any genuinely required parser/build/test validation.
- File read/write/replace actions use optional 1-based inclusive `start_line` / `end_line`; omitted start means 1 and omitted end means EOF. Reads clamp oversized ends; edits reject out-of-file bounds. A trailing LF adds no phantom line and an empty file has zero lines. Range writes preserve the selected closing LF/CRLF when non-empty replacement lacks LF; empty content deletes the range. Whole-file writes create/replace verbatim. Append/prepend require a file (or an earlier whole-file write) and never add implicit newlines.
- Replace uses literal case-sensitive old_text/new_text and mode first|last|all (default first). Matches cannot cross range bounds. All mode processes non-overlapping original matches without searching inserted content again. A missing match fails the whole chain.
- Returned preview data shares 200 lines/16 KiB including line labels, plus bounded factual continuation metadata. File reads stream bounded fragments. Continuations use start_line/start_byte (zero-based bytes within that line); preserve UTF-8 and progress. Listings default to F/D/L/S plus the quoted name, with no stat calls for unused metadata. Optional fields size/permissions/owner/group/modified give byte size, octal mode, numeric UID/GID and UTC time. Keep direct/hidden entries in case-sensitive name order, with a bounded page heap and opaque cursor. Limits default to 100, maximum 200.
- `glob` is a directory discovery action using pinned `github.com/bmatcuk/doublestar/v4` matching. Keep filesystem traversal in the harness: rooted handles, bounded ReadDir batches, cancellation checks, no symlink descent (including literal pattern prefixes), and a page-sized match heap. Do not use an unbounded Glob result slice or GlobWalk brace buffering. Patterns are relative to each selected root; results never expand mutation targets automatically. Default kind is regular file; directory/link/all and up to 16 exclusions are explicit options. Hidden entries are included and .gitignore is not read. Glob shares listing fields and the global output budget; its opaque cursor is query/root-specific and at most 32768 bytes.
- Delete removes regular files, symbolic links themselves, or empty directories. Directory/link deletion is standalone with return summary. File deletion can be staged with other regular-file actions. It is not recursive, never removes the workspace root (including a configured workspace link), and never follows the final symlink. Share entry-path resolution between permission checks and execution; other operations still resolve symlink targets. A final read of a deleted file fails before removal. Summary deletion does not require reading file contents; deletion diffs do.
- Diff comparison caps remain 256 KiB/4000 combined lines after trimming equal outer content; non-text data receives an omission notice. Limits never truncate edits. Validate/prepare output from the exact snapshots before rename; reporting must not reread or turn a committed edit into an error. The tool definition explains explicit output selection; the startup prompt directs the model to that definition and discourages redundant reads. See docs/file-actions.md.
- Control variant: unset `MTT_READ_LINE_NUMBERS` or set it to `false` / `0`. Treatment: set it to `true` / `1`, which returns `N: text` with absolute file line numbers. Labels are not file content. The flag is not a model input; changing it requires restarting the harness.
- Put `MTT_READ_LINE_NUMBERS=true` in `.env` for treatment, or `MTT_READ_LINE_NUMBERS=false` for control, then run `docker compose up --build`. For a new checkout, copy `.env.example` to `.env`. Compose loads the file automatically and forwards the flag; an exported shell value takes precedence. Keep model, prompts, files, and other settings identical between benchmark variants.

## Commands

Install the versioned formatting hook once per checkout:

```sh
./scripts/install-hooks
```

The hook formats staged Go files with `go/format`, root JSON with two-space indentation, and UI code/JSON with the UI's Prettier configuration. Run `npm ci` inside `ui/` before committing UI code. It updates only the staged content and its matching working file; it never stages unrelated edits or permission changes. If a partially staged file needs formatting, the commit stops without changing that file. Format it, then stage the intended hunks again.

Format tracked working files without staging them:

```sh
go run ./scripts/format --all
```

Build and test:

```sh
go build ./...
go test ./...
```

## Configuration

- `mtt.json` is local and gitignored; `mtt.example.json` shows the keys.
- Flags win over the file: `--port`, `--database-url`, `--providers-file`, `--mcp-file`, `--start-prompt-file`, `--test-provider`, `--config`.
- `start_prompt.md` is the shared startup system-prompt template. `start_prompt_file` selects another file; relative paths use the process working directory. Load it once at startup and render one request-local system message before context/request middleware and budgeting. Never persist that generated message in session history or put fallback instructions in provider adapters.
- Prompt variables come from explicit session/runtime values and the authoritative tool registry. `{tool_list}` is a compact registered catalog; `{NAME_info}` renders that tool's description/categories/input schema, including live agent model choices. Rendering never activates tools or exposes search-only documents. Preserve single-pass substitution, `{{variable}}` escaping, recoverable missing-tool errors, and child-session context. See `docs/start-prompt.md`.
- `providers.json` holds the provider URLs, models, prices, and refresh interval. It has no secrets.
- All default provider endpoints, model IDs, capabilities, prices, and model levels belong in `providers.json`, never in Go catalogs. The file ships OpenAI, DeepSeek, and OpenAI ChatGPT coding-plan entries. A missing/empty file injects no providers. Test-provider mode adds its test model alongside file providers.
- Use switch-based protocol/authentication/catalog-format dispatch. A compatible provider is added through JSON, without a provider-name branch. Model IDs and display names are separate; do not infer capabilities from names. Refresh availability follows the remote catalog, including removals and an empty list. Metadata precedence is explicit per-model JSON > provider model-list metadata > configured metadata source > provider-level JSON defaults > cache. Missing tool metadata stays unknown and permits tool requests; explicit `tools:false` suppresses tools. Persist that distinction across restarts. Codex uses its configured `/models` URL and `codex` catalog format, not a static Go model list.
- Models.dev enrichment is selected through `metadata_url`, `metadata_format: models_dev`, and `metadata_provider` in `providers.json`. It supplies capabilities and prices, never availability or credentials. Metadata sources use a 32 MiB limit; malformed/failed refreshes retain the last catalog. Local price overrides win. Billing is `tokens` or `subscription`; subscription models never inherit API token rates. Prices use per-million units and input-context tiers; unknown prices stay unknown, explicit zero stays zero, reasoning tokens are not double billed, provider-reported costs win, and calculated totals preserve `Estimated` through storage.
- Model and reasoning effort belong to one atomic session selection and apply at the next model-request boundary. Supported efforts come from model metadata; empty means the model default. Child agents select their own effort or use their model default. `SessionStore.SetModelSelection` compares the previous selection before changing it; lifecycle `Save` calls must retain both newer fields. Equal/larger context switches are direct. Smaller or previously unknown context needs `allow_compaction` confirmation; target context must be known. Compaction uses whole-turn request fitting and keeps persisted history. Only provider-exposed reasoning text/summaries enter `Message.Reasoning` and chunk events. Encrypted/provider-specific continuation state stays private and replays only to the originating provider.
- Provider tests use synthetic model IDs to verify catalog changes and metadata precedence. Tests that repeat a current production model name/capability do not verify the upstream contract. Verify shipped JSON metadata against authoritative provider sources.
- API keys use the existing database secret store. Coding-plan OAuth access/refresh tokens use `provider_oauth`; public provider/message responses must not expose tokens or internal reasoning continuation data. Device sign-in runs server-side and works from the Docker/Tailscale UI.
- Provider keys are database configuration: `PUT /providers/{id}/key` sets the key for the harness; `PUT /instances/{id}/providers/{provider}/key` sets the key for one instance. The instance key wins.
- Settings are database configuration: `PUT /settings/{key}` sets a value for the harness; `PUT /instances/{id}/settings/{key}` sets a value for one instance. Keys: `agent_depth_limit`, `process_limit`, `api_token`.
- The initial start makes the API token and shows it one time. The user can also set `api_token` in `mtt.json`.
- `mcp.json` lists the MCP servers: `{name, command, args, env, url, headers, enabled}`. It is local and gitignored; `mcp.example.json` shows the keys.

## Docker

Run the harness and Postgres in containers. The standard harness image includes local embeddings. Only `./workspace` is mounted read/write at `/workspace`; it is not the harness repository. Bootstrap, provider/MCP, and startup-prompt files are mounted read-only under `/etc/mtt`.

For a new checkout, prepare the local files and directory:

```sh
mkdir -p workspace
cp -n mtt.example.json mtt.json
cp -n mcp.example.json mcp.json
cp -n .env.example .env
```

Then start Compose:

```sh
docker compose up --build
```

The API is on `http://localhost:18080`. The health path `/health` is open; the other paths need the API token (from the initial-start log or `mtt.json`).

The first image build downloads and verifies the local model assets. Runtime model inference stays inside the harness. Use `/workspace` in the API; host files for a project belong in `./workspace` or a subdirectory there. Keep `workspace/` out of Git and the Docker build context. See `docs/tool-search.md` for core-only builds and the removal procedure.

Run the test suite in containers (the Postgres store tests use the separate, temporary `postgres-test` database; runtime data stays in `postgres`):

```sh
docker compose --profile test run --rm test
```

Stop the containers:

```sh
docker compose down
```

The compose service includes the test provider. Configure OpenAI or DeepSeek in the UI Providers screen, or use the provider-key API. A real provider works without changing Docker flags. Set `MTT_TEST_PROVIDER=false` only to remove the optional test model.

## Runtime Ownership and Recovery

- The optional `plugins/sidekick/` plugin uses typed task-state events, public task/memory/file/model services, and the shared anchored request-reminder boundary. It defaults OFF under `plugin.sidekick`; the bootstrap `sidekick_prompt_file` selects its filtering template. No worker model has a Go default. Event callbacks only coalesce metadata and signal bounded background workers. Retrieval uses current medium-compression memories and rooted regular UTF-8 files; file scans have entry/read/output limits and skip links, hidden/secret paths, dependencies, and build artifacts. Worker output must cite supplied sources and may contain no notes. Fence task/user/config/history changes, recheck source versions, and keep hints as quoted `RoleRuntime` data with stable anchors. Do not create extra user turns, rewrite cached prefixes, or double-bill main-request input. Sidekick owns its schema and joins workers before dependencies close. Its UI configuration and usage live in Settings → Sidekick and the Tasks-panel shortcut.

- The optional `plugins/spaced_repetition/` plugin owns instruction schedules, anchored reminder records, and bounded foreground recovery workers. Bootstrap `spaced_repetition_prompts` selects low.md/medium.md/high.md with the shared startup renderer; behavior and worker-model choices are database settings at `plugin.spaced_repetition`. No worker model has a Go default. Default cadence is 12.5% of model context capped at 32768 tokens, with low/low/low/medium; high is discovery-loaded `remember_instructions` and reads workspace memories at medium compression through the public read-only service.
- Request reminders are projections, not context owners. Count existing anchored reminders during fitting; schedule on the selected context without counting the plugin's own reminders. New additions obey both the model limit and a context policy's stricter ceiling. Freeze rendered bytes and anchors, commit only at dispatch, and fence stale recovery on history/user/config changes. `atom.Message.InContext` is private request metadata so the Responses adapter leaves these system messages in input order rather than folding them into cached startup instructions. Keep worker reference data quoted, never inside assistant prose. Recovery usage belongs to the workspace; reminder tokens already belong to the main request and must not be double billed.

- The optional context plugin is `plugins/context/`. It imports only public harness services and owns the `context_plugin` Postgres schema. It is attached but OFF by default. Settings use the existing database key `plugin.context`, with per-field workspace overrides; worker model IDs have no Go defaults. The plugin API exposes requested/applied state and pending transitions. The UI Memory panel uses these APIs for workspace controls and usage.
- Context preparation and application are separate. `ctx_drop` archives and queues jobs; `ctx_wrapup` applies only prepared reductions at the next request boundary after the complete tool group. Initial session memory is allowed once. Later snapshot text/order/position and L/M/H/I levels change only when content is actually removed. Query results can return fresh data without rewriting that snapshot. Context notices and task-state data stay at the request tail, not in the cached system prefix.
- `remember` saves the exact note and its embeddings synchronously, without a language-model validation or publishing job. Success means saved and immediately searchable. `old_text` selects an exact current note for correction; preserve earlier versions. Current conversation facts are already usable: do not reread memory after saving, narrate memory internals, or claim new facts are unavailable until compaction. Give a brief natural acknowledgement when requested. Context selection references are request-tail metadata, never prefixes inside assistant prose.
- Context workers use the provider registry and workspace allowlists through public services. Their usage has workspace/agent/run/request IDs and no owning chat session; source session IDs are lineage only. Preserve provider costs, estimated costs, unknown usage and subscription billing. Background jobs have durable leases and progress; history mutations and interrupted turns fence stale publication. Shutdown joins workers before embeddings, providers, and storage close.
- `providers.json.context_embeddings` owns local embedding backend/assets/CPU settings separately from `tool_search`; absent configuration selects no memory encoder. Plugin behavior and language-model workers remain database settings. EmbeddingGemma 2 is isolated in `internal/molecule/embedding/embeddinggemma` with one application import in `cmd/mtt/context_embeddings_gemma.go`. It uses pinned Q8 text assets, document/query prefixes, 768 dimensions, an 8192-token ceiling, and a 1024-token chunk budget by default. Keep original text bytes, count prefixes and BOS/EOS, reject NUL/invalid UTF-8 instead of silently truncating, treat media marker strings as literal text, disable native telemetry, and join cancellation before releasing native resources. Query embeddings use the optional public `harness.QueryTextEmbedder` contract. Never mix vector spaces or reset retained data automatically when model assets change.
- PostgreSQL images retain the PostgreSQL 16 Alpine base and add pinned pgvector 0.8.1. Core and plugin migrations use separate transaction-scoped advisory locks. Core schema fingerprints are recorded in `harness_schema_migrations`; unchanged startup must not replay ALTER TABLE statements against active writers. The test database remains separate from runtime data. Do not drop the plugin schema or its archived data during disable or plugin removal.

- Persistent task tracking is per session: `task_state` updates TODO items by stable ID and a nullable DOING title/description. Omitted fields stay unchanged; new items default to pending. Keep done/cancelled items while unfinished items remain, then atomically clear TODO and DOING when all items are terminal. Explicit `todo: [], doing: null` also clears both. Track only when useful; do not create tasks for every message.
- `Store.TaskStates` owns task-state persistence and partial updates. Response counters advance only for completed model responses using the revision observed at the request boundary; updates during a streamed response or its tool group reset freshness. After three responses, add a request-local system reminder asking for a state update first, with discovery first if its schema is absent. The reminder is guidance, not forced tool execution. Task text is quoted data, never instructions; generated snapshots/reminders do not enter saved history. No sidekick or memory-retrieval implementation belongs in this feature.
- `task_state.updated` and `GET /sessions/{id}/task-state` expose the state revision for UI synchronization. Old reads/events must not restore cleared progress or cross sessions. Cancellation/failure clears DOING and retains TODO. Revert clears state with the history change; deletion purges it and rejects late writes through existing session tombstones. Completed child agents clear their own state without changing their parent.

- Parsed complete tool calls start during model streaming. Wait for the whole tool group before the next model request, with all results in model call order. Process observers must not duplicate foreground output into queued messages. Background updates use `atom.RoleRuntime` and `Queue.SubmitRuntimeContent`, never the user-input submission path. Provider adapters encode runtime data as a lower-priority contextual input, not system instructions or a duplicate tool response.

- Session queue state is channel-owned: `Queue.runDirectory` owns coordinator references and lifecycle gates; `sessionCoordinator.runCommandLoop` owns each session's pending list, active queued turn, admission mode, and terminal error. Send commands instead of accessing that state from other goroutines.
- Coordinator handlers must not perform database/network I/O, run models/tools, wait for workers, or invoke plugin callbacks. Those operations run in workers and report completion through bounded channels. Replies have capacity one so abandoned callers do not block owners.
- `Queue.Close` is irreversible once accepted and joins owners and their workers before storage closes. A caller deadline cancels its wait, not the shutdown. Keep memory-store mutexes and Postgres transactions for their separate data-ownership responsibilities.
- The queue persists accepted messages and restores pending work at startup. Limits: 128 waiting messages per session and 4096 total waiting/running messages. Interrupted active turns are reported instead of replayed.
- Instance stop preserves its configuration; `POST /instances/{id}/start` resumes it. Revert fences new submissions and waits for the old writer before deleting history.
- `DELETE /sessions/{id}` deletes an idle conversation tree. The directory/coordinator and loop admission fences protect the tree and its ancestors; unrelated sessions remain responsive. Active turns, pending work, or running processes return 409. Purge messages/events/process records/session permissions, retain usage and workspace approvals, and retain private session tombstones for accounting lineage and stale-write rejection. Do not recreate deleted sessions through Save, Append, queue, event, process, or permission writers. Accepted deletion outlives a caller's wait and joins queue shutdown.
- Model IDs in instance model lists use `provider/model`; bare IDs are accepted only when unambiguous. Allowlists apply to sessions, agents, and request-stage overrides.
- Context budgeting uses `TokenCounter` when provided, otherwise a conservative text-byte/media estimate. Billing uses provider usage, not that estimate.
- `internal/architecture/layers_test.go` checks in-module import boundaries. Go's `internal` visibility rule alone does not block plugins within this module from importing internal packages.
