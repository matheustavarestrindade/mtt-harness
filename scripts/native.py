#!/usr/bin/env python3
"""Build and run the harness/UI on the host, with PostgreSQL in Docker."""

import argparse
import os
from pathlib import Path
import signal
import subprocess
import sys

from native_build import build_native, host_architecture
from native_common import local_path, prepare_bootstrap, read_json
from native_runtime import NativeRuntime


def main():
    parser = argparse.ArgumentParser(prog="./build.sh", description=__doc__)
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--no-build", action="store_true", help="start the existing native build")
    mode.add_argument("--build-only", action="store_true", help="compile without starting services or PostgreSQL")
    mode.add_argument("--stop", action="store_true", help="stop this launcher's user services; keep PostgreSQL/data")
    parser.add_argument("--foreground", action="store_true", help="run in this terminal instead of user services")
    parser.add_argument("--restart", action="store_true", help="replace this launcher's running API/UI services")
    parser.add_argument("--config", default="mtt.json", help="bootstrap file, relative to the checkout")
    parser.add_argument("--state-dir", default="bin/native", help="generated native build/configuration directory")
    parser.add_argument("--assets-dir", help="checksum-verified model/native-library cache (default: STATE_DIR/assets)")
    options = parser.parse_args()
    if sys.version_info < (3, 10):
        parser.error("Python 3.10 or newer is required")
    root = Path(__file__).resolve().parent.parent
    os.chdir(root)
    os.umask(0o077)
    architecture = host_architecture()
    import fcntl
    def interrupt(signum, frame):
        raise KeyboardInterrupt()
    signal.signal(signal.SIGTERM, interrupt)
    state_directory = local_path(root, options.state_dir).resolve()
    configuration_path = local_path(root, options.config)
    state_directory.mkdir(parents=True, exist_ok=True)
    with (state_directory / ".launch.lock").open("a") as launch_lock:
        try:
            fcntl.flock(launch_lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError as failure:
            raise RuntimeError("Another native build/start operation is active in this state directory.") from failure
        runtime = NativeRuntime(root, state_directory, configuration_path)
        if options.stop:
            runtime.stop()
            return
        prepare_bootstrap(root, configuration_path)
        if not options.build_only:
            runtime.prepare(options.restart, options.foreground)
        manifest_path = state_directory / "build.json"
        previous = read_json(manifest_path) if manifest_path.is_file() else {}
        assets = local_path(root, options.assets_dir or previous.get("assets_directory") or str(state_directory / "assets")).resolve()
        if options.no_build:
            if previous.get("platform") != "linux" or previous.get("architecture") != architecture:
                raise RuntimeError("No matching native build exists in this state directory.")
            manifest = previous
        else:
            manifest = build_native(root, state_directory, assets)
        if options.build_only:
            return
        runtime.start(manifest)
        fcntl.flock(launch_lock, fcntl.LOCK_UN)
        runtime.wait_if_foreground()


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print("Native launch interrupted.", file=sys.stderr)
        raise SystemExit(130)
    except (OSError, ValueError, RuntimeError, subprocess.CalledProcessError) as failure:
        print(f"Native startup failed: {failure}", file=sys.stderr)
        if isinstance(failure, subprocess.CalledProcessError) and failure.stderr:
            print(failure.stderr[-16384:], file=sys.stderr)
        raise SystemExit(1)
