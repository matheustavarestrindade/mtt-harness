#!/usr/bin/env bash
set -euo pipefail
project_directory="$(CDPATH= cd -- "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
if ! command -v python3 >/dev/null 2>&1; then
  printf 'Python 3.10 or newer is required for the native build launcher.\n' >&2
  exit 1
fi
exec python3 "$project_directory/scripts/native.py" "$@"
