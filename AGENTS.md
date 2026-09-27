# mtt-harness — Agent Instructions

## Decisions

- Language: Go.
- Plugins: Go packages in the module. An RPC bridge connects an external plugin.
- First user surface: a headless API (HTTP + WebSocket).
- Database: Postgres. Vector recall is deferred to version 2.

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

## Secrets

- `.env` holds the secrets and `.gitignore` keeps it out of the repository.
- `.env.example` documents the keys.
- The harness reads the secrets from the environment.
