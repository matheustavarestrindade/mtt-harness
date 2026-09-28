# mtt-harness — Agent Instructions

## Decisions

- Language: Go.
- Plugins: Go packages in the module. External tools come from MCP servers (`mcp.json`, stdio or HTTP).
- First user surface: a headless API (HTTP + WebSocket).
- Database: Postgres. Vector recall is deferred to version 2.
- Configuration: no `.env`. `mtt.json` is the bootstrap file (port, database URL, file paths, optional token). The database holds the settings and the provider secrets.

## Writing Style

Product documents (`Spec.md`, and later the files in `docs/`) use ASD-STE100 Simplified Technical English.

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

## Go Readability Decisions

- Use complete, descriptive variable, parameter, field, and receiver names. Use `harnessRuntime`, `toolRegistry`, `configuration`, `operationContext`, `operationError`, `request`, and `responseWriter`; do not use `h`, `reg`, `cfg`, `ctx`, `err`, `r`, or `w`. Established domain acronyms such as ID, URL, HTTP, JSON, SQL, and MCP are acceptable.
- Prefer guard clauses and early returns. Handle invalid input and failed operations first; keep the successful path at the outer indentation level. Avoid `else` after a branch that returns.
- Use named error helpers for repeated handling: startup requirements, test assertions, HTTP error responses, and contextual error wrapping. Runtime failures must return errors to their caller. A failed tool, provider, or database operation must not terminate the harness. Do not swallow errors or recover arbitrary programming panics.
- Keep `cmd/mtt/main.go` as a short entry point. Put argument parsing, application wiring, authentication, storage setup, provider loading, and MCP loading in separate, clearly named files.
- Give each file one responsibility. Split files by behavior before adding more unrelated functions; a line-count target must not create meaningless wrapper packages.
- Prefer one authoritative registry or service per capability. Do not add parallel registries or placeholder layers that are disconnected from the runtime.
- Comments explain ownership, ordering, cancellation, and surprising decisions. Do not repeat the code in comments. Exported contracts and concurrency invariants need human-readable documentation.
- Keep refactors separate from functional changes and verify behavior with fresh tests. Passing tests and the STE checker do not establish specification completeness.

## Commands

```sh
go build ./...
go test ./...
```

## Configuration

- `mtt.json` is local and gitignored; `mtt.example.json` shows the keys.
- Flags win over the file: `--port`, `--database-url`, `--providers-file`, `--mcp-file`, `--test-provider`, `--config`.
- `providers.json` holds the provider URLs, models, prices, and refresh interval. It has no secrets.
- Provider keys are database configuration: `PUT /providers/{id}/key` sets the key for the harness; `PUT /instances/{id}/providers/{provider}/key` sets the key for one instance. The instance key wins.
- Settings are database configuration: `PUT /settings/{key}` sets a value for the harness; `PUT /instances/{id}/settings/{key}` sets a value for one instance. Keys: `agent_depth_limit`, `process_limit`, `api_token`.
- The initial start makes the API token and shows it one time. The user can also set `api_token` in `mtt.json`.
- `mcp.json` lists the MCP servers: `{name, command, args, env, url, headers, enabled}`. It is local and gitignored; `mcp.example.json` shows the keys.

## Docker

Run the harness and Postgres in containers (the harness works on this directory through `/workspace`):

```sh
docker compose up --build
```

The API is on `http://localhost:18080`. The health path `/health` is open; the other paths need the API token (from the initial-start log or `mtt.json`).

Run the test suite in containers (the Postgres store tests use the separate, temporary `postgres-test` database; runtime data stays in `postgres`):

```sh
docker compose --profile test run --rm test
```

Stop the containers:

```sh
docker compose down
```

The compose service uses the test provider. For a real model API, run `MTT_TEST_PROVIDER=false docker compose up --build` and set the provider key with the API.

## Runtime Ownership and Recovery

- Session queue state is channel-owned: `Queue.runDirectory` owns coordinator references and lifecycle gates; `sessionCoordinator.run` owns each session's pending list, active queued turn, admission mode, and terminal error. Send commands instead of accessing that state from other goroutines.
- Coordinator handlers must not perform database/network I/O, run models/tools, wait for workers, or invoke plugin callbacks. Those operations run in workers and report completion through bounded channels. Replies have capacity one so abandoned callers do not block owners.
- `Queue.Close` is irreversible once accepted and joins owners and their workers before storage closes. A caller deadline cancels its wait, not the shutdown. Keep memory-store mutexes and Postgres transactions for their separate data-ownership responsibilities.
- The queue persists accepted messages and restores pending work at startup. Limits: 128 waiting messages per session and 4096 total waiting/running messages. Interrupted active turns are reported instead of replayed.
- Instance stop preserves its configuration; `POST /instances/{id}/start` resumes it. Revert fences new submissions and waits for the old writer before deleting history.
- Model IDs in instance model lists use `provider/model`; bare IDs are accepted only when unambiguous. Allowlists apply to sessions, agents, and request-stage overrides.
- Context budgeting uses `TokenCounter` when provided, otherwise a conservative text-byte/media estimate. Billing uses provider usage, not that estimate.
- `internal/architecture/layers_test.go` checks in-module import boundaries. Go's `internal` visibility rule alone does not block plugins within this module from importing internal packages.
