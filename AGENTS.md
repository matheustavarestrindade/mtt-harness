# mtt-harness — Agent Instructions

## Decisions

- Language: Go.
- Plugins: Go packages in the module. External plugins use `plugins.json` and the RPC bridge.
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

## Commands

```sh
go build ./...
go test ./...
```

## Configuration

- `mtt.json` is local and gitignored; `mtt.example.json` shows the keys.
- Flags win over the file: `--port`, `--database-url`, `--providers-file`, `--plugins-file`, `--test-provider`, `--config`.
- `providers.json` holds the provider URLs, models, prices, and refresh interval. It has no secrets.
- Provider keys are database configuration: `PUT /providers/{id}/key` sets the key for the harness; `PUT /instances/{id}/providers/{provider}/key` sets the key for one instance. The instance key wins.
- Settings are database configuration: `PUT /settings/{key}` sets a value for the harness; `PUT /instances/{id}/settings/{key}` sets a value for one instance. Keys: `agent_depth_limit`, `process_limit`, `api_token`.
- The initial start makes the API token and shows it one time. The user can also set `api_token` in `mtt.json`.
- `plugins.json` holds external plugin entries: `{name, command, args, enabled}`.

## Docker

Run the harness and Postgres in containers (the harness works on this directory through `/workspace`):

```sh
docker compose up --build
```

The API is on `http://localhost:18080`. The health path `/health` is open; the other paths need the API token (from the initial-start log or `mtt.json`).

Run the test suite in containers (the Postgres store test uses the container database):

```sh
docker compose --profile test run --rm test
```

Stop the containers:

```sh
docker compose down
```

The compose service uses the test provider. For a real model API, run `MTT_TEST_PROVIDER=false docker compose up --build` and set the provider key with the API.
