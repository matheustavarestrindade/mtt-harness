"""Own native API/UI lifetimes; Docker owns only the existing Postgres service."""

import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import socket
import subprocess
import time
import urllib.parse
import urllib.request

from native_build import check_node, library_environment
from native_common import command, local_path, read_json, require_program, write_private_json


def port_number(value, name, allow_zero=False):
    try:
        number = int(value)
    except (TypeError, ValueError) as failure:
        raise ValueError(f"{name} must be a port number") from failure
    if number < (0 if allow_zero else 1) or number > 65535:
        raise ValueError(f"{name} is outside the port range")
    return number


def require_free_port(number, name):
    if number == 0:
        return
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as probe:
        probe.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        try:
            probe.bind(("127.0.0.1", number))
        except OSError as failure:
            raise RuntimeError(f"{name} port {number} is already in use; no existing process was stopped") from failure


def health_ready(url):
    try:
        with urllib.request.build_opener(urllib.request.ProxyHandler({})).open(url, timeout=2) as response:
            return response.status == 200 and json.load(response).get("status") == "ok"
    except (OSError, ValueError):
        return False


class NativeRuntime:
    def __init__(self, root, state_directory, configuration_path):
        self.root, self.directory, self.configuration_path = root, state_directory, configuration_path
        self.identity = hashlib.sha256(str(state_directory).encode()).hexdigest()[:10]
        self.units = {name: f"mtt-native-{self.identity}-{name}.service" for name in ("api", "ui")}
        self.processes = []
        self.started_units = []
        self.service_mode = False
        self.restart = False

    def systemd_available(self):
        from shutil import which
        return bool(which("systemctl") and which("systemd-run") and
                    command(["systemctl", "--user", "show-environment"], self.root, capture=True, check=False).returncode == 0)

    def unit_properties(self, name):
        result = command(["systemctl", "--user", "show", self.units[name],
                          "--property=LoadState,ActiveState,Description,WorkingDirectory"],
                         self.root, capture=True, check=False)
        return dict(line.split("=", 1) for line in result.stdout.splitlines() if "=" in line)

    def verify_unit_owner(self, name, properties):
        expected = f"mtt native {name} ({self.directory})"
        directory = self.root if name == "api" else self.root / "ui"
        if properties.get("Description") != expected or properties.get("WorkingDirectory") != str(directory):
            raise RuntimeError(f"Native service name is occupied by a different configuration: {self.units[name]}")

    def stop(self):
        if not self.systemd_available():
            print("No user-service manager is available. Foreground runs stop with Ctrl+C.", flush=True)
            return
        stopped = False
        for name in ("ui", "api"):
            properties = self.unit_properties(name)
            if properties.get("LoadState") in (None, "not-found"):
                continue
            self.verify_unit_owner(name, properties)
            command(["systemctl", "--user", "stop", self.units[name]], self.root)
            stopped = True
        print("Native user services stopped. PostgreSQL and saved data remain available." if stopped else
              "No managed native user services. Foreground runs stop with Ctrl+C.", flush=True)

    def prepare(self, restart, foreground):
        require_program("docker")
        check_node(self.root)
        command(["docker", "compose", "version"], self.root, capture=True)
        command(["docker", "info"], self.root, capture=True)
        self.compose = ["docker", "compose", "--project-directory", str(self.root),
                        "--env-file", str(self.root / ".env"), "-f", str(self.root / "compose.yaml"),
                        "-f", str(self.root / "docker/compose.native.yaml")]
        values = command([*self.compose, "config", "--environment"], self.root, capture=True).stdout
        self.values = dict(line.split("=", 1) for line in values.splitlines() if "=" in line)
        compose_configuration = json.loads(command([*self.compose, "config", "--format", "json"], self.root, capture=True).stdout)
        self.database_project = compose_configuration["name"]
        self.database_volume = compose_configuration["volumes"]["pgdata"]["name"]
        self.verify_database_identity()
        self.api_port = port_number(self.values.get("MTT_HOST_PORT") or 18080, "MTT_HOST_PORT")
        self.ui_port = port_number(self.values.get("UI_PORT") or 18081, "UI_PORT")
        self.database_port = port_number(self.values.get("MTT_POSTGRES_PORT") or 15432, "MTT_POSTGRES_PORT", True)
        if self.api_port == self.ui_port or self.database_port in (self.api_port, self.ui_port):
            raise ValueError("API, UI, and PostgreSQL must use different host ports")
        self.ui_bind = self.values.get("UI_BIND_ADDRESS") or "0.0.0.0"
        probe_host = "127.0.0.1" if self.ui_bind in ("0.0.0.0", "::") else self.ui_bind
        if ":" in probe_host and not probe_host.startswith("["):
            probe_host = f"[{probe_host}]"
        self.api_url = f"http://127.0.0.1:{self.api_port}"
        self.ui_url = f"http://{probe_host}:{self.ui_port}"
        self.service_mode = not foreground and self.systemd_available()
        self.restart = restart
        active = set()
        if self.systemd_available():
            for name in self.units:
                properties = self.unit_properties(name)
                if properties.get("LoadState") in (None, "not-found"):
                    continue
                self.verify_unit_owner(name, properties)
                if properties.get("ActiveState") in ("active", "activating", "reloading", "deactivating"):
                    active.add(name)
        if active and not restart:
            raise RuntimeError("This native launcher is already running; --restart is required to replace its services")
        if "api" not in active:
            require_free_port(self.api_port, "API")
        if "ui" not in active:
            require_free_port(self.ui_port, "UI")
        running_harness = command([*self.compose, "ps", "--status", "running", "--quiet", "mtt"], self.root, capture=True).stdout.strip()
        if running_harness:
            raise RuntimeError("A Docker harness is already running for this Compose project")
        published = command([*self.compose, "port", "postgres", "5432"], self.root, capture=True, check=False).stdout.strip()
        if not published.endswith(":" + str(self.database_port)):
            require_free_port(self.database_port, "PostgreSQL")
        read_json(self.configuration_path)

    def verify_database_identity(self):
        previous_path = self.directory / "runtime.json"
        if not previous_path.is_file():
            return
        previous = read_json(previous_path).get("database", {})
        if previous.get("project") != self.database_project:
            return
        result = command(["docker", "volume", "inspect", self.database_volume], self.root, capture=True, check=False)
        if result.returncode != 0:
            raise RuntimeError("The saved PostgreSQL volume is unavailable in the current Docker context")
        current = json.loads(result.stdout)[0]
        if previous.get("volume") != self.database_volume or previous.get("created_at") != current["CreatedAt"]:
            raise RuntimeError("The PostgreSQL volume identity differs from the saved native installation")

    def prepare_runtime_files(self, manifest, container):
        original = read_json(self.configuration_path)
        providers_path = local_path(self.root, original.get("providers_file") or "providers.json")
        providers = read_json(providers_path) if providers_path.is_file() else {}
        assets = Path(manifest["assets_directory"])
        # Remap only shipped container paths. Custom provider/asset configuration
        # stays authoritative, and relative custom paths still use the checkout.
        remaps = [("tool_search", "model_directory", "/opt/mtt/models/all-MiniLM-L6-v2", assets / "models/all-MiniLM-L6-v2"),
                  ("context_embeddings", "model_directory", "/opt/mtt/models/embeddinggemma-2", assets / "models/embeddinggemma-2"),
                  ("context_embeddings", "runtime_library", "/opt/mtt/lib/libonnxruntime.so.1.30.0", assets / "lib/libonnxruntime.so.1.30.0")]
        for section, field, shipped, native in remaps:
            if isinstance(providers.get(section), dict) and providers[section].get(field) == shipped:
                providers[section][field] = str(native)
        write_private_json(self.directory / "providers.json", providers)
        database_environment = dict(value.split("=", 1) for value in container["Config"]["Env"] if "=" in value)
        bindings = container["NetworkSettings"]["Ports"].get("5432/tcp") or []
        binding = next((value for value in bindings if value["HostIp"] == "127.0.0.1"), None)
        if not binding:
            raise RuntimeError("PostgreSQL has no loopback host port for the native harness")
        def quote(value):
            return urllib.parse.quote(value, safe="")
        original["database_url"] = (f"postgres://{quote(database_environment['POSTGRES_USER'])}:"
                                    f"{quote(database_environment['POSTGRES_PASSWORD'])}@127.0.0.1:{binding['HostPort']}/"
                                    f"{quote(database_environment['POSTGRES_DB'])}?sslmode=disable")
        original["port"] = self.api_port
        original["providers_file"] = str(self.directory / "providers.json")
        for key, default in [("mcp_file", "mcp.json"), ("start_prompt_file", "start_prompt.md"),
                             ("spaced_repetition_prompts", "plugins/spaced_repetition/prompts"),
                             ("sidekick_prompt_file", "plugins/sidekick/prompt.md")]:
            original[key] = str(local_path(self.root, original.get(key) or default))
        if "MTT_TEST_PROVIDER" in self.values:
            value = self.values["MTT_TEST_PROVIDER"].lower()
            if value not in ("true", "false", "1", "0", ""):
                raise ValueError("MTT_TEST_PROVIDER must be true/false or 1/0")
            original["test_provider"] = value in ("true", "1")
        write_private_json(self.directory / "mtt.json", original)

    def start(self, manifest):
        binary = Path(manifest.get("binary") or self.directory / "mtt")
        ui_directory = Path(manifest["ui_directory"])
        if not binary.is_file() or not (ui_directory / "index.html").is_file():
            raise RuntimeError("Native binary or UI build is missing; run build.sh without --no-build")
        node = check_node(self.root)
        vite = self.root / "ui/node_modules/vite/bin/vite.js"
        if not vite.is_file():
            raise RuntimeError("UI dependencies are missing; run npm ci inside ui")
        if self.restart:
            self.stop()
        require_free_port(self.api_port, "API")
        require_free_port(self.ui_port, "UI")
        print("Starting PostgreSQL in Docker…", flush=True)
        command([*self.compose, "up", "--detach", "--build", "--wait", "--wait-timeout", "180", "postgres"], self.root)
        identifier = command([*self.compose, "ps", "--quiet", "postgres"], self.root, capture=True).stdout.strip()
        if not identifier:
            raise RuntimeError("PostgreSQL container is unavailable")
        container = json.loads(command(["docker", "inspect", identifier], self.root, capture=True).stdout)[0]
        volume = json.loads(command(["docker", "volume", "inspect", self.database_volume], self.root, capture=True).stdout)[0]
        self.prepare_runtime_files(manifest, container)
        compiler = shlex.split(os.environ.get("CC", "gcc"))
        from shutil import which
        if not compiler or not which(compiler[0]):
            compiler = None
        api_environment, _ = library_environment(self.root, compiler, manifest.get("native_library_directories", []))
        if "MTT_READ_LINE_NUMBERS" in self.values:
            api_environment["MTT_READ_LINE_NUMBERS"] = self.values["MTT_READ_LINE_NUMBERS"]
        ui_environment = dict(os.environ, HARNESS_API_URL=self.api_url)
        commands = {"api": [str(binary), "--config", str(self.directory / "mtt.json")],
                    "ui": [node, str(vite), "preview", "--host", self.ui_bind, "--port", str(self.ui_port),
                           "--strictPort", "--outDir", str(ui_directory)]}
        self.logs = {name: self.directory / (name + ".log") for name in commands}
        offsets = {name: path.stat().st_size if path.exists() else 0 for name, path in self.logs.items()}
        try:
            for name, arguments in commands.items():
                directory = self.root if name == "api" else self.root / "ui"
                environment = api_environment if name == "api" else ui_environment
                with self.logs[name].open("a"):
                    self.logs[name].chmod(0o600)
                if self.service_mode:
                    properties = ["WorkingDirectory=" + str(directory), "Restart=on-failure", "RestartSec=2",
                                  "TimeoutStopSec=60", "StandardOutput=append:" + str(self.logs[name]),
                                  "StandardError=inherit"]
                    launch = ["systemd-run", "--user", "--collect", "--unit=" + self.units[name],
                              f"--description=mtt native {name} ({self.directory})"]
                    launch += ["--property=" + value for value in properties]
                    # Copy named variables from the client environment. Values,
                    # which can include credentials, must not enter argv.
                    for key in sorted(environment):
                        if re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", key):
                            launch.append("--setenv=" + key)
                    self.started_units.append(name)
                    command([*launch, *arguments], self.root, environment)
                else:
                    with self.logs[name].open("a") as output:
                        self.processes.append(subprocess.Popen(arguments, cwd=directory, env=environment,
                                                               stdout=output, stderr=subprocess.STDOUT, start_new_session=True))
            self.wait_for_health()
            write_private_json(self.directory / "runtime.json", {"root": str(self.root), "api_port": self.api_port,
                               "ui_port": self.ui_port, "service_mode": self.service_mode, "units": self.units,
                               "database": {"project": self.database_project, "volume": self.database_volume,
                                            "created_at": volume["CreatedAt"]}})
        except BaseException:
            self.stop_started()
            self.show_log_tail(offsets)
            raise
        self.show_log_tail({"api": offsets["api"]})
        print(f"\nNative API: {self.api_url}\nNative UI: {self.ui_url}\nLogs: {self.directory}\n"
              "Workspaces use host paths. Agent commands inherit the host PATH and user permissions.", flush=True)
        if self.service_mode:
            print("User services are running in the background. Use build.sh --stop to stop them.\n"
                  "After a reboot, build.sh --no-build starts them again.", flush=True)
        else:
            print("Running in this terminal. Ctrl+C stops the API and UI; PostgreSQL stays running.", flush=True)

    def wait_for_health(self):
        deadline = time.monotonic() + 180
        while time.monotonic() < deadline:
            if any(process.poll() is not None for process in self.processes):
                raise RuntimeError("A native process exited before startup completed")
            if health_ready(self.api_url + "/health") and health_ready(self.ui_url + "/api/health"):
                return
            time.sleep(0.25)
        raise RuntimeError("The native API/UI health check did not become ready within 180 seconds")

    def show_log_tail(self, offsets):
        for name, offset in offsets.items():
            path = self.logs[name]
            if not path.is_file():
                continue
            with path.open("rb") as stream:
                stream.seek(max(offset, path.stat().st_size - 65536))
                text = stream.read(65536).decode("utf-8", errors="replace")
            if text:
                print(text, end="" if text.endswith("\n") else "\n", flush=True)

    def stop_started(self):
        for name in reversed(self.started_units):
            command(["systemctl", "--user", "stop", self.units[name]], self.root, check=False)
        self.started_units.clear()
        for process in self.processes:
            if process.poll() is None:
                process.terminate()
        for process in self.processes:
            try:
                process.wait(timeout=60)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
        self.processes.clear()

    def wait_if_foreground(self):
        if self.service_mode:
            return
        try:
            while all(process.poll() is None for process in self.processes):
                time.sleep(0.25)
            raise RuntimeError("A native process exited; stopping the other process")
        except KeyboardInterrupt:
            pass
        finally:
            self.stop_started()
