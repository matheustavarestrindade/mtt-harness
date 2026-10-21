"""Local file and command operations for the native launcher."""

import json
import os
from pathlib import Path
import shutil
import signal
import subprocess


def command(arguments, root, environment=None, capture=False, check=True):
    arguments = [str(value) for value in arguments]
    process = subprocess.Popen(arguments, cwd=root, env=environment, text=True, start_new_session=True,
                               stdout=subprocess.PIPE if capture else None, stderr=subprocess.PIPE if capture else None)
    try:
        output, errors = process.communicate()
    except BaseException:
        try:
            os.killpg(process.pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
        try:
            process.wait(timeout=30)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            process.wait()
        raise
    result = subprocess.CompletedProcess(arguments, process.returncode, output, errors)
    if check:
        result.check_returncode()
    return result


def require_program(name):
    executable = shutil.which(name)
    if not executable:
        raise RuntimeError(f"Required host program is not installed: {name}")
    return executable


def local_path(root, name):
    path = Path(name)
    return path if path.is_absolute() else root / path


def read_json(path):
    with Path(path).open(encoding="utf-8") as stream:
        value = json.load(stream)
    if not isinstance(value, dict):
        raise ValueError(f"Expected a JSON object: {path}")
    return value


def write_private_json(path, value):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_name(path.name + f".next-{os.getpid()}")
    try:
        with temporary.open("w", encoding="utf-8") as stream:
            temporary.chmod(0o600)
            json.dump(value, stream, indent=2)
            stream.write("\n")
        temporary.replace(path)
    finally:
        temporary.unlink(missing_ok=True)


def prepare_bootstrap(root, configuration_path):
    for path, example in [(configuration_path, root / "mtt.example.json"),
                          (root / "mcp.json", root / "mcp.example.json"),
                          (root / ".env", root / ".env.example")]:
        if not path.exists() and not path.is_symlink():
            try:
                with path.open("xb") as destination, example.open("rb") as source:
                    shutil.copyfileobj(source, destination)
                print(f"Created {path.relative_to(root) if path.is_relative_to(root) else path}", flush=True)
            except FileExistsError:
                pass
        if not path.is_file():
            raise RuntimeError(f"Configuration is not a regular file: {path}")
