"""Build the full host binary and UI with checksum-verified local embeddings."""

import hashlib
import os
from pathlib import Path
import platform
import re
import shlex
import shutil
import sys
import tempfile

from native_common import command, require_program, write_private_json


def host_architecture():
    if platform.system() != "Linux":
        raise RuntimeError("The full native launcher currently supports Linux (including WSL2).")
    architecture = {"x86_64": "amd64", "aarch64": "arm64", "arm64": "arm64"}.get(platform.machine())
    if not architecture:
        raise RuntimeError(f"Unsupported native architecture: {platform.machine()}")
    return architecture


def check_node(root):
    executable = require_program("node")
    version = command([executable, "--version"], root, capture=True).stdout.strip()
    match = re.fullmatch(r"v(\d+)\.(\d+)\.(\d+)", version)
    if not match or tuple(map(int, match.groups())) < (22, 12, 0):
        raise RuntimeError(f"Node.js 22.12 or newer is required; found {version}")
    return executable


def library_environment(root, compiler=None, retained=()):
    environment = dict(os.environ)
    directories = []
    for directory in retained:
        if Path(directory).is_dir() and directory not in directories:
            directories.append(directory)
    if compiler:
        for library in ("libstdc++.so.6", "libgomp.so.1"):
            result = command([*compiler, "-print-file-name=" + library], root, capture=True, check=False)
            path = Path(result.stdout.strip())
            if result.returncode == 0 and path.is_absolute() and path.is_file():
                directory = str(path.parent)
                if directory not in directories:
                    directories.append(directory)
    if directories:
        environment["LD_LIBRARY_PATH"] = os.pathsep.join(directories + ([environment["LD_LIBRARY_PATH"]] if environment.get("LD_LIBRARY_PATH") else []))
    return environment, directories


def build_native(root, state_directory, assets_directory):
    architecture = host_architecture()
    require_program("go")
    require_program("npm")
    check_node(root)
    compiler = shlex.split(os.environ.get("CC") or command(["go", "env", "CC"], root, capture=True).stdout.strip())
    if not compiler:
        raise RuntimeError("A host C/C++ toolchain is required for the native embedding adapters.")
    require_program(compiler[0])
    print("Verifying local model and native-library assets…", flush=True)
    for script, arguments in [
        ("download-tool-search-model.py", [assets_directory / "models/all-MiniLM-L6-v2"]),
        ("download-context-model.py", [assets_directory / "models/embeddinggemma-2"]),
        ("download-embedding-runtime.py", [architecture, assets_directory / "lib"]),
    ]:
        command([sys.executable, root / "scripts" / script, *arguments], root)
    environment, native_directories = library_environment(root, compiler)
    environment["CGO_ENABLED"] = "1"
    linker_flag = shlex.quote("-L" + str(assets_directory / "lib"))
    environment["CGO_LDFLAGS"] = (environment.get("CGO_LDFLAGS", "") + " " + linker_flag).strip()
    state_directory.mkdir(parents=True, exist_ok=True)
    (state_directory / "builds").mkdir(exist_ok=True)
    build_directory = Path(tempfile.mkdtemp(prefix="build-", dir=state_directory / "builds"))
    binary = build_directory / "mtt"
    suffix = hashlib.sha256(str(state_directory).encode()).hexdigest()[:10]
    ui_directory = root / "ui/dist" / ("native-" + suffix) / build_directory.name
    try:
        print("Building the native harness…", flush=True)
        command(["go", "build", "-tags", "semantic,gemma", "-trimpath", "-o", binary, "./cmd/mtt"], root, environment)
        print("Building the standalone UI…", flush=True)
        command(["npm", "ci"], root / "ui")
        command(["npm", "run", "check"], root / "ui")
        command(["npm", "run", "build", "--", "--outDir", ui_directory, "--emptyOutDir"], root / "ui")
    except BaseException:
        shutil.rmtree(build_directory)
        if ui_directory.exists():
            shutil.rmtree(ui_directory)
        raise
    manifest = {"platform": "linux", "architecture": architecture,
                "binary": str(binary),
                "assets_directory": str(assets_directory), "ui_directory": str(ui_directory),
                "native_library_directories": native_directories}
    write_private_json(state_directory / "build.json", manifest)
    binary_link = state_directory / f"mtt.next-{os.getpid()}"
    try:
        binary_link.symlink_to(binary.relative_to(state_directory))
        binary_link.replace(state_directory / "mtt")
    finally:
        binary_link.unlink(missing_ok=True)
    print(f"Native binary: {state_directory / 'mtt'}", flush=True)
    return manifest
