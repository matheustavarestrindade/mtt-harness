# mtt-harness

A Go harness with one agent loop, workspace instances, tools, plugins, and an HTTP/WebSocket API. The Svelte UI is an API client.

## Native Installation

The `build.sh` file starts a native installation on Linux. The harness and UI run on the host. PostgreSQL runs in Docker.

```text
Host requirements:
  Linux amd64 or arm64 (WSL2 is also a Linux host)
  Bash, Python 3.10+, Go 1.26+, Node.js 22.12+ and npm
  C/C++ toolchain with libstdc++ and libgomp
  Docker Engine with Compose v2.20+; daemon running
  Optional: a systemd user session for background services
```

```sh
git clone https://github.com/matheustavarestrindade/mtt-harness.git
cd mtt-harness
./build.sh
```

The initial native build gets model assets and compares SHA-256 values. The asset cache keeps model assets. The procedure prepares new configuration files. It does not replace local files or database data.

```text
UI:  http://localhost:18081
API: http://localhost:18080
UI connection: /api
PostgreSQL: 127.0.0.1:15432
Provider setup: Configuration → Settings → Providers
```

The harness makes an `API token` on the initial start. The token is in the harness output. The test provider is off by default.

Workspaces use host paths, for example `/home/me/projects/project`. Commands use the host `PATH` and user permissions. A bind mount is not necessary.

## Commands

```sh
# Start the saved native build after a reboot:
./build.sh --no-build

# Build an update and restart this launcher's services:
./build.sh --restart

# Compile without starting the application or database:
./build.sh --build-only

# Run in the terminal; Ctrl+C stops the native processes:
./build.sh --foreground

# Stop this launcher's user services; keep PostgreSQL and data:
./build.sh --stop
```

The native launcher uses user services when available. If user services are not available, it runs in the terminal. It does not stop a different program that uses the same ports.

See `docs/native-start.md` for configuration and host requirements.

## Docker

The full Docker deployment is also available:

```sh
./start.sh
```

The `start.sh` procedure runs the harness, PostgreSQL, and UI in containers. Commands run in the container. Host project directories use bind mounts. See `docs/docker-start.md`.

## Project Files

- `Spec.md`: harness requirements.
- `routes.md`: API data.
- `AGENTS.md`: project rules.
- `ui/AGENTS.md`: UI rules.

```sh
go build ./...
go test ./...
```
