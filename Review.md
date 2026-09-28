# Harness implementation review

Reviewed baseline: `ebf7802` on `master`.

File/line references in the findings refer to that baseline. The readability refactor moved functions into focused files without changing behavior. The subsequent functional pass closes the fifteen findings below and adds permanent regression coverage.

The original Docker suite and Go vet passed despite these defects. Fifteen temporary review probes exposed them. The corrected behavior is now checked by repository regression tests across the real runtime, HTTP/MCP transports, and Postgres.

## Resolution and evidence

| Findings | Correction | Regression coverage |
|---|---|---|
| 1–2 | Shared workspace resolution, workspace shell directory, instance-scoped session queries | `internal/tools/workspace_test.go`, `internal/api/runtime_regression_test.go` |
| 3–4 | Deny precedes approvals; one-time decisions are not reused; stored grants are scoped by instance/session/target | `internal/molecule/permission/engine_test.go`, `internal/organism/loop/runtime_regression_test.go` |
| 5–6 | Complete tool calls start during streaming; revert fences admission and joins the old writer before deleting history | `internal/organism/loop/runtime_regression_test.go` |
| 7 | Application-owned MCP connections and cleanup | `cmd/mtt/mcp_test.go` |
| 8 | Stable registration IDs and copied callback snapshots | `harness/registrations_test.go` |
| 9 | JSON Schema validation, including null, enums, references and additional properties | `internal/molecule/schema/regression_test.go` |
| 10 | Correct Postgres aggregate arguments and released query connections | `internal/molecule/store/postgres/runtime_regression_test.go` |
| 11 | Drained, bounded retained output and late terminal-event replay | `internal/molecule/process/lifecycle_test.go` |
| 12 | Live instance settings override migrated creation values | `internal/api/runtime_regression_test.go` |
| 13 | One runtime tool/provider catalog and shared event handlers | `internal/organism/loop/runtime_regression_test.go` |
| 14 | Configuration prices replace stale cached prices | `internal/molecule/provider/regression_test.go` |
| 15 | Process observers own a lifetime independent of the originating turn | `internal/molecule/store/postgres/runtime_regression_test.go` |

The additional gaps are also addressed: durable bounded queues and interrupted-turn recovery; stop/resume without configuration loss; model allowlists and provider-qualified IDs; whole-turn context budgeting; multimodal request/result preservation; responsive MCP catalog refresh with pagination; live API-token rotation; persisted event sequences and WebSocket replay; process-group termination; local-config Docker exclusions; corrected interfaces and an in-module architecture test.

Context budgeting uses a provider `TokenCounter` when available. Its fallback is a conservative text/media estimate, not provider billing usage. M6/vector recall remains deferred as agreed.

## Channel-owned session queues

The follow-up on PR #1 replaces the queue's shared mutex and worker wait group with an owning directory goroutine and one command-processing goroutine per session. `sessionCoordinator.run` is the single entry point for state transitions, with named handlers for submit, cancellation, revert, stop/resume, and shutdown.

The model and database workers do not mutate coordinator state. They send results through completion channels. Public requests use cancellation-aware sends and single-slot reply channels. A status read returns one detached snapshot instead of reading running, pending, and error fields separately. Coordinator handlers execute neither I/O nor plugin callbacks.

Shutdown fences admission and joins accepted operations, including a database commit that finishes after the requesting caller leaves. Idle coordinators remain available until queue shutdown. The loop's low-level execution guard and tool-group synchronization, the in-memory store's mutex, and Postgres transactions retain their distinct responsibilities.

`internal/organism/loop/coordinator_concurrency_test.go` covers blocked persistence without blocking cancellation/status or unrelated sessions, abandoned replies, shutdown waiting for late commits, callback status queries, and instance stop overtaking an unfinished revert without reopening admission.

Verification includes the container build, Postgres-backed regression suite, Go vet, race checks, and STE validation of `Spec.md`. A live container smoke test covered queue processing, instance isolation, statistics, stop/resume, revert, and conversation continuation. Restarting that container also preserved its instances, sessions, and statistics.

## Original confirmed findings

1. **Workspace operations use the server directory.** The path guard checks a path relative to the instance workspace, but `read`, `write`, and `bash` execute relative to the server working directory. A probe approved an instance file and read the server's file instead. Baseline references: `plugins/pathtools/pathtools.go:40`, `internal/tools/read.go:40`, `internal/tools/write.go:41`, `internal/tools/bash.go:55`.
2. **Session lists cross instance boundaries.** `GET /instances/{id}/sessions` calls the global `Agents("")` query. Both stores return all root sessions. A request for instance A returned instance B's session. References: `internal/api/server.go:227`, `internal/organism/memory/session.go:48`.
3. **Cached allow overrides explicit deny.** `Loop.allow` consults the cache before checking `deny`. A denied tool executed after an earlier cached approval. Reference: `internal/organism/loop/loop.go:288`.
4. **Permission scopes are not enforced.** Decisions are cached by target alone, including one-time decisions. A one-time approval was reused by another session in another instance. References: `internal/molecule/permission/engine.go:31`, `internal/organism/loop/loop.go:314`.
5. **Tools wait for the entire model stream.** Complete calls are collected while streaming, but execution starts only after EOF and message persistence. A stream waiting for its first complete tool to start timed out. This does not satisfy Spec R4. Reference: `internal/organism/loop/loop.go:110-169`.
6. **Revert does not wait for the old writer.** Queue clearing sends cancellation and immediately returns. The API then deletes history while the old run can still append. The memory backend probe wrote two messages after revert. References: `internal/organism/loop/queue.go:127`, `internal/api/server.go:377`.
7. **MCP clients close during startup.** `loadMCP` defers connection cleanup inside the loader; its return closes every registered client before the API starts. Calling a registered tool returned `mcp: the connection is closed`. Reference: `cmd/mtt/main.go:255`.
8. **Event unsubscription removes the wrong listener.** Closures capture slice indexes that change when earlier listeners are removed. Removing listeners zero and one left listener one instead of listener two. The same pattern exists in the plugin event, middleware, decision, and watcher registries. References: `internal/molecule/eventbus/eventbus.go:49`, `harness/harness.go:36`.
9. **Schema checking is incomplete and fails open.** The validator accepts null for a required string, invalid enum members, and forbidden additional properties. Unsupported or malformed schemas disable validation. Reference: `internal/molecule/schema/schema.go`.
10. **Harness-wide Postgres statistics fail.** The no-placeholder aggregate query receives one nil argument. The real database returned `expected 0 arguments, got 1`. Reference: `internal/molecule/store/postgres/postgres.go:457-466`.
11. **Completed process output disappears.** The supervisor removes completed handles, but `bash` and `process_output` retrieve output through those handles. Output was unavailable immediately after completion. References: `internal/molecule/process/supervisor.go:234`, `internal/tools/bash.go:97`.
12. **Instance setting changes can be ignored.** A limit stored in the instance creation record takes precedence over a later instance settings update. Updating the depth setting to five left the effective limit at two. Reference: `internal/organism/instances/manager.go:51`.
13. **Plugin registrations do not reach runtime services.** `Harness.Tool` and `Harness.Provider` maintain separate maps from the loop registry and gateway. `Harness.On` subscribes to a different event collection from the runtime event bus. A registered plugin tool never ran and an event handler never received model events. References: `harness/harness.go`, `cmd/mtt/main.go`, `internal/organism/loop/loop.go`.
14. **Price edits can lose to stale cached data.** Startup loads configured prices before cached models; refresh preserves existing prices when the endpoint provides none. A configured price of two remained the cached price of one. References: `cmd/mtt/main.go:213-222`, `internal/molecule/provider/standard.go:184-200`.
15. **Background process records and notifications use an expired run context.** The queue cancels the context when a turn ends, but observers continue using it for Postgres writes. A completed process remained recorded as running. References: `internal/organism/loop/queue.go:76`, `internal/organism/processes/manager.go:74,188,224`.

## Original additional gaps

- Context-window limits are stored but never enforced or used to trim context.
- The HTTP message API accepts text only. Audio/file content becomes text in the standard adapter, and tool results are flattened to text by the loop.
- Model allowlists are applied to model listings, not to execution. Model IDs are not qualified by provider in the gateway, so two providers can collide.
- The MCP tools-changed callback runs on the response-reader goroutine and calls `ListTools`, preventing that reader from receiving its own response until timeout.
- Updating the database API token does not update the token captured by the running HTTP server.
- The queue is memory-only, unbounded, and has no shutdown drain or durable acknowledgement. Runtime failures are logged without a terminal failure event for the client.
- Stopping an instance deletes its configuration, does not cancel its queued/running sessions, and leaves those sessions addressable.
- Event sequence numbers come from two independent counters: the event bus and the database. Restart and concurrent writes can make streamed and persisted sequence numbers disagree.
- Process subscription, completion, and output-reader lifetimes are not coordinated. Fast processes can finish before observers subscribe. Killing a shell does not establish process-group termination.
- `.dockerignore` excludes obsolete environment files but does not exclude local `mtt.json` or `mcp.json`, which are copied into the builder/test image.
- The Spec contains duplicate plugin requirements and outdated interface declarations. Its claim that Go prevents in-module plugins from importing `internal` packages is too broad.
- M6/vector recall remains intentionally deferred.

## Readability assessment

The repository has a useful foundation: ordinary Go, explicit contracts, small domain types, recognizable HTTP handlers, and replaceable stores. The main maintenance problems are abbreviated names, unrelated responsibilities collected in large files, repeated error-handling boilerplate, ignored errors, and disconnected duplicate services.

The readability refactor preserves the atoms/molecules/organisms design, replaces abbreviated identifiers, separates source files by behavior, and adds named startup, HTTP, test, and error-context helpers. `cmd/mtt/main.go` is now 25 lines. The specification and agent instructions record these decisions.

The naming and file-ownership rules remain in force during the functional changes. Green tests are evidence for the listed behaviors, not a blanket claim that every future integration or provider is complete.
