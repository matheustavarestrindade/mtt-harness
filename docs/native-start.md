# Native Installation

## Configuration

The `build.sh` file makes a native build of the harness and UI. PostgreSQL runs through Docker Compose. Database data is in `pgdata`.

```text
Native API: Go executable with semantic + gemma build tags
Native UI: built files and the Node/Vite HTTP/WebSocket proxy
Database: PostgreSQL 16 + pgvector in Docker
Workspaces: host paths
Commands: host PATH and user permissions
```

## Requirements

```text
Platform: Linux amd64 or arm64, including WSL2
Not supported by this native-library procedure: macOS / native Windows

Bash
Python 3.10+
Go 1.26+ (go.mod selects the required toolchain)
Node.js 22.12+ and npm
C/C++ toolchain with libstdc++ and libgomp
Docker Engine and Compose v2.20+, with the local daemon running

Optional: systemctl --user and systemd-run --user
Compiler selection: CC, otherwise go env CC
NixOS: compiler library directories are added to the launch environment
```

The procedure does not use `sudo` or install `apt` packages. Commands use user permissions.

## Commands

```sh
# Build and start; user services are preferred when available:
./build.sh

# Start the saved build after a reboot:
./build.sh --no-build

# Build an update and restart this launcher's user services:
./build.sh --restart

# Restart without a build:
./build.sh --no-build --restart

# Compile without starting PostgreSQL or native processes:
./build.sh --build-only

# Run in the terminal; Ctrl+C stops both native processes:
./build.sh --foreground

# Stop managed user services, retaining PostgreSQL and its data:
./build.sh --stop
```

The user services are temporary. The command is `./build.sh --no-build` after the computer starts again. A foreground run uses the terminal. PostgreSQL is not stopped by `--stop` or `Ctrl+C`.

## Source Files and Ports

New local files use `mtt.example.json`, `mcp.example.json`, and `.env.example`. The procedure does not replace local configuration files. It keeps database data and workspace paths.

```dotenv
# Host environment values override .env values.
MTT_HOST_PORT=18080
UI_PORT=18081
UI_BIND_ADDRESS=0.0.0.0
MTT_POSTGRES_PORT=15432

# Optional separate database project:
# COMPOSE_PROJECT_NAME=my-harness

MTT_READ_LINE_NUMBERS=true
MTT_TEST_PROVIDER=false
```

Docker Compose reads `.env`. The native launcher does not run `.env` as shell code. PostgreSQL is available only on `127.0.0.1`.

The native launcher sets the API port and database URL in `bin/native/mtt.json`. The source `mtt.json` does not change. A different bootstrap file can be selected with `--config FILE`.

Provider URLs, models, and prices stay in the selected providers file. The `bin/native/providers.json` file changes the default `/opt/mtt` asset paths to host paths. User paths and provider settings do not change. The source files selected by `mcp_file`, `start_prompt_file`, `spaced_repetition_prompts`, and `sidekick_prompt_file` stay the same.

Provider secrets and runtime settings stay in PostgreSQL. Use `Configuration → Settings → Providers` in the UI. The initial harness output contains the new `API token`.

```text
Generated configuration and log-file permissions: 0600
Database identity: Compose project, volume name, and volume creation time
Missing/replaced saved database volume: stop with an error
Automatic database restore/reset: none
Workspace path conversion: none
```

## Build Output

```text
bin/native/build.json       build manifest
bin/native/mtt              link to latest native binary
bin/native/builds/          versioned native binaries
bin/native/assets/          checksum-verified model/library cache
bin/native/mtt.json         generated bootstrap file
bin/native/providers.json  generated provider file
bin/native/api.log          native API output
bin/native/ui.log           native UI output
ui/dist/native-*/           versioned UI builds

--state-dir DIR: generated build/configuration directory
--assets-dir DIR: model/library cache
Relative paths: based on the checkout
Absolute paths: accepted
Build failure: retain previous published build and running UI
Cached assets: reuse only after SHA-256 verification
Model inference: in-process, inside the harness
```

The model assets and SHA-256 checks are the same for `build.sh` and `start.sh`. A native build has a new build directory. The UI files are in `ui/dist`.

## Program Start

The native launcher gives an error if a different program uses the same port. Stop the previous program, or select different ports. Stop the Docker harness for the same Compose project before the native harness.

Use host paths in the workspace form. The `MTT_WORKSPACE_DIR` and `MTT_HOST_ROOT` values apply to the Docker deployment. The native launcher does not change instance configuration.

## Test Suite

```sh
go test ./scripts -run TestNativeLauncherContracts -v
MTT_TEST_NATIVE_LAUNCHER=1 go test ./scripts -run TestNativeBuildAndLaunch -v -timeout=20m
```

The test suite uses different ports and a different Compose project. It examines host Python access, `PATH`, `UID`, file writes, the UI proxy, WebSockets, and database data after a new start.
