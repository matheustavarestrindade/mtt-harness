# Docker Start

## Requirements

Docker and Bash are necessary. The Docker daemon must be active.

```text
Docker Engine with the Compose plugin, or Docker Desktop
Docker Compose: v2.20 or newer, with --wait and --wait-timeout
Shell: Bash on Linux, macOS, WSL2, or Git Bash
Containers: Linux
Embedding runtime: in-process, inside the harness container
```

The image build gets model assets and compares SHA-256 values. The harness does not get model assets at runtime.

## Start

```sh
./start.sh
./start.sh --no-build
./start.sh --help
```

The `start.sh` file can start from a different working directory. The `.env` paths use the repository directory. The procedure does not replace local configuration files. The default workspace directory is also prepared.

The start procedure examines PostgreSQL, the harness, and the UI. The UI proxy uses `http://mtt:8080`. The browser uses `/api`.

The harness makes an `API token` on the initial database start. The `start.sh` output includes the initial harness output. The database keeps the `API token` and data. `api_token` in `mtt.json` can set a different `API token`.

## Configuration

Docker Compose reads `.env`. Shell environment values apply before file values. The Go process does not read `.env`.

```dotenv
MTT_HOST_PORT=18080
UI_PORT=18081
UI_BIND_ADDRESS=0.0.0.0
MTT_WORKSPACE_DIR=./workspace
MTT_TEST_PROVIDER=false
MTT_READ_LINE_NUMBERS=true

# Optional:
# MTT_HOST_ROOT=/
# MTT_UID=1000
# MTT_GID=1000
# COMPOSE_PROJECT_NAME=my-harness
```

The container uses the host user and group IDs unless `MTT_UID` and `MTT_GID` give different IDs. The procedure does not change the UID or GID of host files.

Provider model lists stay in `providers.json`. Provider keys, OAuth tokens, and runtime settings stay in PostgreSQL. `mtt.json` gives the database connection and file paths. `mcp.json` is local MCP configuration.

## Host Files

The default bind mount is:

```text
Host ./workspace -> container /workspace
```

Set `MTT_WORKSPACE_DIR` to a host parent directory for different projects. The host directory must be available. Files written at `/workspace/project` are host files.

`MTT_HOST_ROOT` adds `docker/compose.host-files.yaml` and a read/write bind mount at `/host`. The default does not include the bind mount. The workspace API receives container paths.

```dotenv
# Linux example for full host-file access:
MTT_HOST_ROOT=/
```

```text
Host /home/me/projects/example
Container /host/home/me/projects/example
```

Docker Desktop file permissions also apply. The container does not automatically get host programs, processes, or the Docker socket. Put necessary project tools and `stdio` MCP programs in the container image. Host HTTP servers can use `host.docker.internal`.

## Data

Database data is in `pgdata`. Host project files stay in bind mounts.

```sh
# Containers use restart: unless-stopped.
docker compose stop
./start.sh --no-build
docker compose logs --tail 100 mtt
```

The `start.sh` file does not remove `pgdata` or database data. It does not use a database backup or stop a different Compose project.

```text
Data scope: Docker context + COMPOSE_PROJECT_NAME
```

A repository on a different computer does not include `pgdata`. Use a database backup to move database data. Examine the source data and the new database before a data change.

```text
Do not use docker compose down --volumes when database data must be kept.
```

## Test Suite

```sh
go test ./scripts
MTT_TEST_START_DOCKER=1 go test ./scripts -run TestDockerStartLifecycle -v -timeout=30m
```

The test uses a different Compose project and available local ports. It examines a new database, API authentication, HTTP and WebSocket proxy access, and host files. It also examines database data after stop/start and the optional host bind mount.
