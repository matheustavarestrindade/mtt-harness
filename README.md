# mtt-harness

A Go harness with one agent loop, workspace instances, tools, plugins, and an HTTP/WebSocket API. The Svelte UI is an API client.

## Start

Docker and Bash are necessary on the host. `Go`, `Node.js`, and `Python` are not necessary for the start procedure.

```text
Linux: Docker Engine + Docker Compose v2.20 or newer
macOS: Docker Desktop + Bash
Windows: Docker Desktop + WSL2 or Git Bash
```

Start the Docker daemon. Run the command:

```sh
git clone https://github.com/matheustavarestrindade/mtt-harness.git
cd mtt-harness
./start.sh
```

The `start.sh` file starts PostgreSQL, the harness, and the UI. The image build gets model assets and compares SHA-256 values. The harness makes an `API token` on the initial start. The `API token` is in the harness output.

```text
UI:  http://localhost:18081
API: http://localhost:18080
UI connection: /api
Provider setup: Configuration → Settings → Providers
```

The test provider is off by default. The `start.sh` file prepares new local configuration files. It does not replace local files or database data.

```sh
# Start images without an image build:
./start.sh --no-build
```

## Host Files

Workspace files stay on the host. The default host directory is `./workspace`. It is available at `/workspace` in the container.

```sh
mkdir -p workspace/project
# Use /workspace/project in the workspace form.
```

Set a different host directory in `.env`:

```dotenv
MTT_WORKSPACE_DIR=/home/me/projects
```

Then `/workspace/project` is the host directory `/home/me/projects/project`. `MTT_HOST_ROOT` adds a bind mount at `/host`. On Linux, `/` gives access to the host filesystem. File permissions apply.

```dotenv
MTT_HOST_ROOT=/
# A workspace can then use /host/home/me/projects/project.
```

Commands run in the container. Host file access does not install host programs in the container. See `docs/docker-start.md` for configuration and data.

## Stop

```sh
docker compose stop
```

Run `./start.sh` again to start the containers. Docker also starts the containers after a program start, unless the user stopped the containers.

## Project Files

- `Spec.md`: harness requirements.
- `routes.md`: API data.
- `AGENTS.md`: project rules.
- `ui/AGENTS.md`: UI rules.

```sh
go build ./...
go test ./...
```
