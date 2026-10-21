#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'HELP'
Usage: ./start.sh [--no-build] [--help]

Build and start PostgreSQL, the harness, and its web UI with Docker Compose.
Missing mtt.json, mcp.json, and .env files are copied from the examples.
Existing files, workspace contents, and database volumes are preserved.

  --no-build  Start existing images without rebuilding them.
  --help      Show this help without starting Docker.

Deployment settings belong in .env or the invoking environment:
  MTT_WORKSPACE_DIR  Host directory mounted at /workspace (default: ./workspace).
  MTT_HOST_ROOT      Optional extra host directory mounted at /host.
  MTT_HOST_PORT     API host port (default: 18080).
  UI_PORT           Web UI host port (default: 18081).
  MTT_UID/MTT_GID   Container user/group (default: the invoking user's IDs).

Examples:
  ./start.sh
  MTT_HOST_PORT=28080 UI_PORT=28081 ./start.sh
  MTT_WORKSPACE_DIR=/home/me/projects ./start.sh
  MTT_HOST_ROOT=/ ./start.sh

Put persistent deployment choices in .env for later invocations.
HELP
}

build_arguments=(--build)
for argument in "$@"; do
  case "$argument" in
    --help|-h) usage; exit 0 ;;
    --no-build) build_arguments=(--no-build) ;;
    *) printf 'Unknown start.sh option: %s\n' "$argument" >&2; usage >&2; exit 2 ;;
  esac
done

project_directory="$(CDPATH= cd -- "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
cd "$project_directory"

if ! command -v docker >/dev/null 2>&1; then
  printf 'Docker with the Compose plugin is required.\n' >&2
  exit 1
fi
docker compose version >/dev/null
docker info >/dev/null
compose_help="$(docker compose up --help)"
if [[ "$compose_help" != *--wait-timeout* ]]; then
  printf 'Docker Compose with --wait and --wait-timeout is required (v2.20 or newer).\n' >&2
  exit 1
fi

# Bootstrap is additive. Never replace a local file, follow a dangling link to
# create a configuration, or change ownership of an existing workspace.
umask 077
for target in mtt.json mcp.json .env; do
  case "$target" in
    mtt.json) example=mtt.example.json ;;
    mcp.json) example=mcp.example.json ;;
    .env) example=.env.example ;;
  esac
  if [[ ! -e "$target" && ! -L "$target" ]]; then
    cp -n "$example" "$target"
    printf 'Created %s from %s.\n' "$target" "$example"
  fi
  if [[ ! -f "$target" || ! -r "$target" ]]; then
    printf 'Configuration file is not a readable file: %s\n' "$target" >&2
    exit 1
  fi
done
mkdir -p workspace

compose_command=(docker compose --project-directory "$project_directory" --env-file "$project_directory/.env" -f "$project_directory/compose.yaml")
# Compose owns dotenv syntax and precedence. The optional override references
# MTT_HOST_ROOT while inspecting configuration; it is not started unless enabled.
compose_environment="$("${compose_command[@]}" -f "$project_directory/docker/compose.host-files.yaml" config --environment)"
configured_value() {
  local name="$1" line
  while IFS= read -r line; do
    case "$line" in "$name="*) printf '%s' "${line#*=}"; return 0 ;; esac
  done <<< "$compose_environment"
}

if [[ -z "$(configured_value MTT_UID)" ]]; then export MTT_UID="$(id -u)"; fi
if [[ -z "$(configured_value MTT_GID)" ]]; then export MTT_GID="$(id -g)"; fi
workspace_directory="$(configured_value MTT_WORKSPACE_DIR)"
workspace_directory="${workspace_directory:-./workspace}"
if [[ ! -d "$workspace_directory" ]]; then
  printf 'Workspace mount source is not a directory: %s\n' "$workspace_directory" >&2
  exit 1
fi
host_directory="$(configured_value MTT_HOST_ROOT)"
if [[ -n "$host_directory" ]]; then
  if [[ ! -d "$host_directory" ]]; then
    printf 'Host mount source is not a directory: %s\n' "$host_directory" >&2
    exit 1
  fi
  compose_command+=(-f "$project_directory/docker/compose.host-files.yaml")
fi
"${compose_command[@]}" config --quiet
previous_container="$("${compose_command[@]}" ps --all --quiet mtt)"
printf 'Starting PostgreSQL, harness, and UI. The first build downloads verified model assets.\n'
if ! "${compose_command[@]}" up --detach --wait --wait-timeout 180 "${build_arguments[@]}" postgres mtt ui; then
  "${compose_command[@]}" ps --all >&2 || true
  "${compose_command[@]}" logs --no-color --tail 80 mtt ui >&2 || true
  exit 1
fi
"${compose_command[@]}" exec -T ui wget -qO- http://127.0.0.1:8080/api/health >/dev/null

api_address="$("${compose_command[@]}" port mtt 8080)"
ui_address="$("${compose_command[@]}" port ui 8080)"
printf '\nUI:  http://localhost:%s\nAPI: http://localhost:%s\n' "${ui_address##*:}" "${api_address##*:}"
printf 'Use the Docker host address instead of localhost from another device.\n'
printf 'Workspace mount: %s -> /workspace\n' "$workspace_directory"
if [[ -n "$host_directory" ]]; then printf 'Additional host mount: %s -> /host\n' "$host_directory"; fi
printf 'Provider keys and runtime settings are configured in the UI.\n'
if [[ -z "$previous_container" ]]; then
  printf '\nInitial harness output (a newly initialized database prints its API token here):\n'
  "${compose_command[@]}" logs --no-color mtt
else
  printf 'The existing API token is retained. Logs: docker compose logs mtt\n'
fi
