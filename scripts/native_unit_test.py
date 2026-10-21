"""Native launcher contracts that do not require a Docker daemon."""

import contextlib
import hashlib
import io
import json
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import unittest
from unittest.mock import patch
from urllib.parse import urlparse, unquote

from native_common import prepare_bootstrap, local_path
from native_build import build_native
from native_runtime import NativeRuntime, port_number, require_free_port
from verified_download import download_verified


class NativeLauncherTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="mtt native test ")
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.state = self.root / "bin/native"
        self.state.mkdir(parents=True)

    def test_bootstrap_preserves_existing_configuration_and_links(self):
        for target, example in [("mtt.json", "mtt.example.json"), ("mcp.json", "mcp.example.json"), (".env", ".env.example")]:
            (self.root / example).write_text("example " + target)
        (self.root / "mtt.json").write_text("private existing config")
        prepare_bootstrap(self.root, self.root / "mtt.json")
        self.assertEqual((self.root / "mtt.json").read_text(), "private existing config")
        self.assertEqual((self.root / "mcp.json").read_text(), "example mcp.json")
        (self.root / "mcp.json").unlink()
        (self.root / "mcp.json").symlink_to(self.root / "absent")
        with self.assertRaises(RuntimeError):
            prepare_bootstrap(self.root, self.root / "mtt.json")
        self.assertFalse((self.root / "absent").exists())
        self.assertEqual(local_path(self.root, "~/config.json"), self.root / "~/config.json")

    def test_runtime_uses_host_paths_and_private_derived_configuration(self):
        original = {"api_token": "test-token", "port": 8080, "providers_file": "providers.json", "mcp_file": "my-mcp.json", "start_prompt_file": "my-prompt.md", "test_provider": True}
        providers = {"providers": [{"name": "synthetic", "api_url": "http://example.invalid"}],
                     "tool_search": {"mode": "auto", "model_directory": "/opt/mtt/models/all-MiniLM-L6-v2"},
                     "context_embeddings": {"model_directory": "custom/model", "runtime_library": "/opt/mtt/lib/libonnxruntime.so.1.30.0"}}
        (self.root / "mtt.json").write_text(json.dumps(original))
        (self.root / "providers.json").write_text(json.dumps(providers))
        runtime = NativeRuntime(self.root, self.state, self.root / "mtt.json")
        runtime.api_port, runtime.values = 23456, {"MTT_TEST_PROVIDER": "false"}
        container = {"Config": {"Env": ["POSTGRES_USER=mtt user", "POSTGRES_PASSWORD=p@ss/#?", "POSTGRES_DB=mtt db"]},
                     "NetworkSettings": {"Ports": {"5432/tcp": [{"HostIp": "127.0.0.1", "HostPort": "23457"}]}}}
        runtime.prepare_runtime_files({"assets_directory": str(self.state / "assets")}, container)
        configuration = json.loads((self.state / "mtt.json").read_text())
        self.assertEqual(configuration["port"], 23456)
        self.assertEqual(configuration["api_token"], "test-token")
        self.assertFalse(configuration["test_provider"])
        self.assertEqual(configuration["mcp_file"], str(self.root / "my-mcp.json"))
        self.assertEqual(configuration["start_prompt_file"], str(self.root / "my-prompt.md"))
        connection = urlparse(configuration["database_url"])
        self.assertEqual(unquote(connection.password), "p@ss/#?")
        self.assertEqual(connection.hostname, "127.0.0.1")
        self.assertEqual(connection.port, 23457)
        derived = json.loads((self.state / "providers.json").read_text())
        self.assertEqual(derived["tool_search"]["model_directory"], str(self.state / "assets/models/all-MiniLM-L6-v2"))
        self.assertEqual(derived["context_embeddings"]["model_directory"], "custom/model")
        self.assertEqual(json.loads((self.root / "mtt.json").read_text()), original)
        self.assertEqual(json.loads((self.root / "providers.json").read_text()), providers)
        self.assertEqual((self.state / "mtt.json").stat().st_mode & 0o777, 0o600)

    def test_foreign_services_are_not_stopped(self):
        runtime = NativeRuntime(self.root, self.state, self.root / "mtt.json")
        with patch.object(runtime, "systemd_available", return_value=True), patch.object(runtime, "unit_properties", return_value={"LoadState": "loaded", "Description": "another application", "WorkingDirectory": str(self.root)}), patch("native_runtime.command") as execute:
            with self.assertRaisesRegex(RuntimeError, "different configuration"):
                runtime.stop()
            execute.assert_not_called()

    def test_startup_diagnostics_keep_the_error_before_ui_log_creation(self):
        runtime = NativeRuntime(self.root, self.state, self.root / "mtt.json")
        runtime.logs = {"api": self.state / "api.log", "ui": self.state / "ui.log"}
        runtime.logs["api"].write_text("startup failed before UI launch\n")
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            runtime.show_log_tail({"api": 0, "ui": 0})
        self.assertIn("startup failed before UI launch", output.getvalue())

    def test_first_launch_reads_the_effective_database_token_without_rotating_it(self):
        runtime = NativeRuntime(self.root, self.state, self.root / "mtt.json")
        container = {"Id": "native-postgres", "Config": {"Env": ["POSTGRES_USER=custom user", "POSTGRES_DB=custom database"]}}
        database = {"project": "test", "volume": "test_pgdata", "created_at": "first"}
        # A reused database may have a different token from an earlier log entry.
        (self.state / "api.log").write_text("mtt API token: obsolete-token\n")
        result = subprocess.CompletedProcess([], 0, json.dumps("current-token") + "\n", "")
        with patch("native_runtime.command", return_value=result) as execute:
            self.assertEqual(runtime.read_first_launch_token(container, database), "current-token")
        arguments = execute.call_args.args[0]
        self.assertEqual(arguments[:3], ["docker", "exec", "native-postgres"])
        self.assertEqual(arguments[arguments.index("--username") + 1], "custom user")
        self.assertEqual(arguments[arguments.index("--dbname") + 1], "custom database")
        self.assertEqual(arguments[-1], "SELECT to_json(value) FROM settings WHERE scope = '' AND key = 'api_token'")
        self.assertNotIn("current-token", arguments)
        self.assertFalse((self.state / "runtime.json").exists())

    def test_token_display_is_once_per_current_database_and_upgrades_legacy_state(self):
        runtime = NativeRuntime(self.root, self.state, self.root / "mtt.json")
        database = {"project": "test", "volume": "test_pgdata", "created_at": "first"}
        container = {"Id": "native-postgres", "Config": {"Env": ["POSTGRES_USER=mtt", "POSTGRES_DB=mtt"]}}
        result = subprocess.CompletedProcess([], 0, '"current-token"\n', "")
        for displayed in (None, False):
            previous = {"database": database}
            if displayed is not None:
                previous["api_token_displayed"] = displayed
            (self.state / "runtime.json").write_text(json.dumps(previous))
            with patch("native_runtime.command", return_value=result) as execute:
                self.assertEqual(runtime.read_first_launch_token(container, database), "current-token")
                execute.assert_called_once()
        (self.state / "runtime.json").write_text(json.dumps({"database": database, "api_token_displayed": True}))
        with patch("native_runtime.command") as execute:
            self.assertIsNone(runtime.read_first_launch_token(container, database))
            execute.assert_not_called()
        different_database = {"project": "different", "volume": "different_pgdata", "created_at": "second"}
        with patch("native_runtime.command", return_value=result):
            self.assertEqual(runtime.read_first_launch_token(container, different_database), "current-token")
            self.assertEqual(runtime.read_first_launch_token(container, dict(database, name="different-database")), "current-token")

    def test_unavailable_token_does_not_record_success_or_generate_another_token(self):
        runtime = NativeRuntime(self.root, self.state, self.root / "mtt.json")
        container = {"Id": "native-postgres", "Config": {"Env": ["POSTGRES_USER=mtt", "POSTGRES_DB=mtt"]}}
        database = {"project": "test", "volume": "test_pgdata", "created_at": "first"}
        for value in ("", "null", '""', "42", "[]"):
            with self.subTest(value=value), patch("native_runtime.command", return_value=subprocess.CompletedProcess([], 0, value, "")):
                with self.assertRaisesRegex(RuntimeError, "did not return an API token"):
                    runtime.read_first_launch_token(container, database)
            self.assertFalse((self.state / "runtime.json").exists())

    def test_startup_log_excerpt_does_not_duplicate_the_token(self):
        runtime = NativeRuntime(self.root, self.state, self.root / "mtt.json")
        runtime.logs = {"api": self.state / "api.log"}
        for prefix in ("", "x" * 70000 + "\n"):
            runtime.logs["api"].write_text(prefix + "mtt API token: current-token\nready\n")
            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                runtime.show_log_tail({"api": 0}, include_api_token=False)
            self.assertEqual(output.getvalue(), "ready\n")

    def test_display_marker_follows_successful_output_and_contains_no_token(self):
        runtime = NativeRuntime(self.root, self.state, self.root / "mtt.json")
        runtime.api_url, runtime.ui_url = "http://127.0.0.1:18080", "http://127.0.0.1:18081"
        state = {"database": {"project": "test"}, "api_token_displayed": False}
        state_path = self.state / "runtime.json"
        state_path.write_text(json.dumps(state))

        def failing_output(message, **options):
            if "API token:" in message:
                raise BrokenPipeError("output closed")

        with patch("builtins.print", side_effect=failing_output):
            with self.assertRaises(BrokenPipeError):
                runtime.show_connection_summary(state, "current-token")
        self.assertFalse(json.loads(state_path.read_text())["api_token_displayed"])
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            runtime.show_connection_summary(state, "current-token")
        self.assertIn("API token: current-token", output.getvalue())
        self.assertIn("UI API address: /api", output.getvalue())
        self.assertTrue(json.loads(state_path.read_text())["api_token_displayed"])
        self.assertNotIn("current-token", state_path.read_text())

    def test_busy_ports_are_not_taken_over(self):
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            listener.listen()
            with self.assertRaisesRegex(RuntimeError, "already in use"):
                require_free_port(listener.getsockname()[1], "API")
            with socket.create_connection(listener.getsockname(), timeout=1):
                pass
        for value in ("127.0.0.1:18080", "-1", "65536", "0"):
            with self.assertRaises(ValueError):
                port_number(value, "API")
        self.assertEqual(port_number("0", "Database", True), 0)

    def test_missing_or_replaced_saved_database_is_not_reinitialized(self):
        runtime = NativeRuntime(self.root, self.state, self.root / "mtt.json")
        runtime.database_project, runtime.database_volume = "test-project", "test-project_pgdata"
        (self.state / "runtime.json").write_text(json.dumps({"database": {"project": runtime.database_project, "volume": runtime.database_volume, "created_at": "original"}}))
        with patch("native_runtime.command", return_value=subprocess.CompletedProcess([], 1, "", "volume missing")):
            with self.assertRaisesRegex(RuntimeError, "unavailable"):
                runtime.verify_database_identity()
        with patch("native_runtime.command", return_value=subprocess.CompletedProcess([], 0, '[{"CreatedAt":"replacement"}]', "")):
            with self.assertRaisesRegex(RuntimeError, "identity differs"):
                runtime.verify_database_identity()
        with patch("native_runtime.command", return_value=subprocess.CompletedProcess([], 0, '[{"CreatedAt":"original"}]', "")):
            runtime.verify_database_identity()

    def test_asset_cache_is_verified_and_failed_download_is_atomic(self):
        source, target = self.root / "source", self.root / "asset"
        source.write_bytes(b"first verified asset")
        expected = hashlib.sha256(source.read_bytes()).hexdigest()
        download_verified(source.as_uri(), target, expected)
        source.unlink()
        download_verified(source.as_uri(), target, expected)
        source.write_bytes(b"corrupt replacement")
        with self.assertRaisesRegex(RuntimeError, "checksum mismatch"):
            download_verified(source.as_uri(), target, hashlib.sha256(b"expected replacement").hexdigest())
        self.assertEqual(target.read_bytes(), b"first verified asset")
        self.assertFalse(list(self.root.glob("*.download-*")))

    def test_help_does_not_require_docker(self):
        launcher = Path(__file__).resolve().parents[1] / "build.sh"
        result = subprocess.run(["bash", str(launcher), "--help"], cwd=self.root, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("--foreground", result.stdout)
        self.assertIn("--build-only", result.stdout)

    def test_failed_build_keeps_published_artifacts(self):
        previous = self.state / "old-mtt"
        previous.write_bytes(b"previous binary")
        (self.state / "mtt").symlink_to(previous)
        original_manifest = '{"binary":"previous-build"}\n'
        (self.state / "build.json").write_text(original_manifest)

        def execute(arguments, root, environment=None, capture=False, check=True):
            arguments = [str(value) for value in arguments]
            if arguments[:3] == ["go", "env", "CC"]:
                return subprocess.CompletedProcess(arguments, 0, "gcc\n", "")
            if arguments[:2] == ["go", "build"]:
                Path(arguments[arguments.index("-o") + 1]).write_bytes(b"unpublished binary")
            if arguments[:3] == ["npm", "run", "check"]:
                raise subprocess.CalledProcessError(1, arguments)
            return subprocess.CompletedProcess(arguments, 0, "", "")

        with patch("native_build.command", side_effect=execute), patch("native_build.require_program", return_value="gcc"), patch("native_build.check_node"), patch("native_build.host_architecture", return_value="amd64"), patch("native_build.library_environment", return_value=({}, [])):
            with self.assertRaises(subprocess.CalledProcessError):
                build_native(self.root, self.state, self.state / "assets")
        self.assertEqual((self.state / "build.json").read_text(), original_manifest)
        self.assertEqual((self.state / "mtt").read_bytes(), b"previous binary")
        self.assertEqual(list((self.state / "builds").iterdir()), [])


if __name__ == "__main__":
    unittest.main()
