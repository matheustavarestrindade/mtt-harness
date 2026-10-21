package scripts_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

const mockDocker = `#!/usr/bin/env bash
set -eu
printf '%s\0' "$@" >> "$START_TEST_CALLS"
printf '\0' >> "$START_TEST_CALLS"
arguments=" $* "
if [[ "$arguments" == *' up --help '* ]]; then printf '%s\n' '--wait --wait-timeout'; exit 0; fi
if [[ "$arguments" == *' config --environment '* ]]; then printf '%s\n' "${START_TEST_ENVIRONMENT:-}"; exit 0; fi
if [[ "$arguments" == *' ps --all --quiet mtt '* ]]; then [[ ! -f "$START_TEST_RUNNING" ]] || printf '%s\n' 'test-container'; exit 0; fi
if [[ "$arguments" == *' up --detach '* ]]; then
  printf '%s:%s' "${MTT_UID:-unset}" "${MTT_GID:-unset}" > "$START_TEST_IDENTITIES"
  touch "$START_TEST_RUNNING"
  exit "${START_TEST_UP_EXIT:-0}"
fi
if [[ "$arguments" == *' port mtt 8080 '* ]]; then printf '127.0.0.1:28080\n'; exit 0; fi
if [[ "$arguments" == *' port ui 8080 '* ]]; then printf '127.0.0.1:28081\n'; exit 0; fi
if [[ "$arguments" == *' logs '* ]]; then printf 'mtt API token: synthetic-start-token\n'; exit 0; fi
if [[ "$arguments" == *' exec -T ui '* ]]; then printf '{"status":"ok"}'; exit 0; fi
`

type startFixture struct {
	directory   string
	outside     string
	calls       string
	running     string
	identities  string
	environment []string
}

func makeStartFixture(testingHandle *testing.T) startFixture {
	testingHandle.Helper()
	if runtime.GOOS == "windows" {
		testingHandle.Skip("start.sh uses Bash; run these checks in WSL")
	}
	if _, operationError := exec.LookPath("bash"); operationError != nil {
		testingHandle.Skip("Bash is not installed")
	}
	_, sourceFile, _, found := runtime.Caller(0)
	if !found {
		testingHandle.Fatal("test source path is unavailable")
	}
	root := filepath.Dir(filepath.Dir(sourceFile))
	temporary := testingHandle.TempDir()
	fixture := startFixture{directory: filepath.Join(temporary, "checkout with spaces"), outside: filepath.Join(temporary, "another directory"), calls: filepath.Join(temporary, "docker-calls"), running: filepath.Join(temporary, "running"), identities: filepath.Join(temporary, "identities")}
	for _, directory := range []string{fixture.directory, fixture.outside, filepath.Join(temporary, "bin"), filepath.Join(fixture.directory, "docker"), filepath.Join(fixture.directory, "ui")} {
		testutil.RequireNoError(testingHandle, os.MkdirAll(directory, 0700))
	}
	for _, name := range []string{"start.sh", "mtt.example.json", "mcp.example.json", ".env.example", "compose.yaml", "docker/compose.host-files.yaml", "ui/compose.yaml"} {
		content, operationError := os.ReadFile(filepath.Join(root, name))
		testutil.RequireNoError(testingHandle, operationError)
		testutil.RequireNoError(testingHandle, os.WriteFile(filepath.Join(fixture.directory, name), content, 0700))
	}
	testutil.RequireNoError(testingHandle, os.WriteFile(filepath.Join(temporary, "bin", "docker"), []byte(mockDocker), 0700))
	for _, variable := range os.Environ() {
		name, _, _ := strings.Cut(variable, "=")
		if name == "PATH" || strings.HasPrefix(name, "MTT_") || strings.HasPrefix(name, "UI_") || strings.HasPrefix(name, "COMPOSE_") || strings.HasPrefix(name, "START_TEST_") {
			continue
		}
		fixture.environment = append(fixture.environment, variable)
	}
	fixture.environment = append(fixture.environment, "PATH="+filepath.Join(temporary, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"), "START_TEST_CALLS="+fixture.calls, "START_TEST_RUNNING="+fixture.running, "START_TEST_IDENTITIES="+fixture.identities)
	return fixture
}

func (fixture startFixture) run(arguments ...string) (string, error) {
	command := exec.Command("bash", append([]string{filepath.Join(fixture.directory, "start.sh")}, arguments...)...)
	command.Dir = fixture.outside
	command.Env = fixture.environment
	output, operationError := command.CombinedOutput()
	return string(output), operationError
}

func TestStartBootstrapsWithoutReplacingLocalFiles(testingHandle *testing.T) {
	fixture := makeStartFixture(testingHandle)
	output, operationError := fixture.run()
	testutil.RequireNoError(testingHandle, operationError)
	if !strings.Contains(output, "http://localhost:28081") || !strings.Contains(output, "synthetic-start-token") {
		testingHandle.Fatalf("initial startup output: %s", output)
	}
	for _, pair := range [][2]string{{"mtt.json", "mtt.example.json"}, {"mcp.json", "mcp.example.json"}, {".env", ".env.example"}} {
		actual, readError := os.ReadFile(filepath.Join(fixture.directory, pair[0]))
		testutil.RequireNoError(testingHandle, readError)
		expected, readError := os.ReadFile(filepath.Join(fixture.directory, pair[1]))
		testutil.RequireNoError(testingHandle, readError)
		if string(actual) != string(expected) {
			testingHandle.Fatalf("bootstrap file differs: %s", pair[0])
		}
	}
	identity, operationError := os.ReadFile(fixture.identities)
	testutil.RequireNoError(testingHandle, operationError)
	if string(identity) != strconv.Itoa(os.Getuid())+":"+strconv.Itoa(os.Getgid()) {
		testingHandle.Fatalf("default ownership: %s", identity)
	}
	testutil.RequireNoError(testingHandle, os.WriteFile(filepath.Join(fixture.directory, "mtt.json"), []byte("existing local configuration\n"), 0600))
	testutil.RequireNoError(testingHandle, os.WriteFile(filepath.Join(fixture.directory, "workspace", "keep.txt"), []byte("host project"), 0600))
	output, operationError = fixture.run("--no-build")
	testutil.RequireNoError(testingHandle, operationError)
	if strings.Contains(output, "synthetic-start-token") {
		testingHandle.Fatal("a repeated startup displayed the previous initial token")
	}
	retained, operationError := os.ReadFile(filepath.Join(fixture.directory, "mtt.json"))
	testutil.RequireNoError(testingHandle, operationError)
	if string(retained) != "existing local configuration\n" {
		testingHandle.Fatal("local configuration was replaced")
	}
	retained, operationError = os.ReadFile(filepath.Join(fixture.directory, "workspace", "keep.txt"))
	testutil.RequireNoError(testingHandle, operationError)
	if string(retained) != "host project" {
		testingHandle.Fatal("host workspace content changed")
	}
	calls, operationError := os.ReadFile(fixture.calls)
	testutil.RequireNoError(testingHandle, operationError)
	for _, forbidden := range []string{"\x00down\x00", "\x00rm\x00", "\x00--volumes\x00"} {
		if strings.Contains(string(calls), forbidden) {
			testingHandle.Fatalf("destructive startup operation: %q", forbidden)
		}
	}
	if !strings.Contains(string(calls), "\x00--no-build\x00") || !strings.Contains(string(calls), fixture.directory+"\x00") {
		testingHandle.Fatal("arguments or checkout path lost their boundaries")
	}
	for _, call := range strings.Split(string(calls), "\x00\x00") {
		if strings.Contains(call, "\x00up\x00") && !strings.Contains(call, "\x00--help") && strings.Contains(call, "compose.host-files.yaml") {
			testingHandle.Fatal("host-root mount was enabled without configuration")
		}
	}
}

func TestStartHonorsComposeOwnershipAndHostMountConfiguration(testingHandle *testing.T) {
	fixture := makeStartFixture(testingHandle)
	fixture.environment = append(fixture.environment, "MTT_UID=2401", "MTT_GID=2402", "START_TEST_ENVIRONMENT=MTT_UID=2401\nMTT_GID=2402\nMTT_WORKSPACE_DIR="+fixture.outside+"\nMTT_HOST_ROOT="+fixture.outside)
	output, operationError := fixture.run()
	testutil.RequireNoError(testingHandle, operationError)
	identity, operationError := os.ReadFile(fixture.identities)
	testutil.RequireNoError(testingHandle, operationError)
	if string(identity) != "2401:2402" {
		testingHandle.Fatalf("configured ownership changed: %s", identity)
	}
	if !strings.Contains(output, fixture.outside+" -> /host") {
		testingHandle.Fatalf("host mount is not reported: %s", output)
	}
	calls, operationError := os.ReadFile(fixture.calls)
	testutil.RequireNoError(testingHandle, operationError)
	found := false
	for _, call := range strings.Split(string(calls), "\x00\x00") {
		if strings.Contains(call, "\x00up\x00") && strings.Contains(call, "compose.host-files.yaml") {
			found = true
		}
	}
	if !found {
		testingHandle.Fatal("configured host mount was not included in startup")
	}
}

func TestStartFailsWithoutResettingData(testingHandle *testing.T) {
	fixture := makeStartFixture(testingHandle)
	fixture.environment = append(fixture.environment, "START_TEST_UP_EXIT=1")
	output, operationError := fixture.run()
	if operationError == nil || strings.Contains(output, "UI:  http://") {
		testingHandle.Fatalf("failed startup reported success: %s", output)
	}
	if _, operationError = os.Stat(filepath.Join(fixture.directory, "mtt.json")); operationError != nil {
		testingHandle.Fatal("failure removed bootstrap configuration")
	}
	calls, operationError := os.ReadFile(fixture.calls)
	testutil.RequireNoError(testingHandle, operationError)
	if strings.Contains(string(calls), "\x00down\x00") {
		testingHandle.Fatal("failed startup shut down existing services")
	}
}

func TestStartRejectsInvalidPathsAndOptionsBeforeLaunch(testingHandle *testing.T) {
	fixture := makeStartFixture(testingHandle)
	if _, operationError := fixture.run("--wipe"); operationError == nil {
		testingHandle.Fatal("unsupported destructive option accepted")
	}
	if _, operationError := os.Stat(filepath.Join(fixture.directory, "mtt.json")); !os.IsNotExist(operationError) {
		testingHandle.Fatal("invalid option changed local files")
	}
	fixture.environment = append(fixture.environment, "START_TEST_ENVIRONMENT=MTT_WORKSPACE_DIR="+filepath.Join(fixture.outside, "missing"))
	if _, operationError := fixture.run(); operationError == nil {
		testingHandle.Fatal("missing workspace mount source accepted")
	}
	if _, operationError := os.Stat(fixture.running); !os.IsNotExist(operationError) {
		testingHandle.Fatal("containers started with an invalid mount source")
	}
}

func TestComposeUsesIndependentUIAndAutomaticNetworks(testingHandle *testing.T) {
	if _, operationError := exec.LookPath("docker"); operationError != nil {
		testingHandle.Skip("Docker Compose is not installed")
	}
	_, sourceFile, _, _ := runtime.Caller(0)
	root := filepath.Dir(filepath.Dir(sourceFile))
	command := exec.Command("docker", "compose", "--project-directory", root, "--env-file", os.DevNull, "-f", filepath.Join(root, "compose.yaml"), "config", "--format", "json")
	output, operationError := command.CombinedOutput()
	if operationError != nil {
		testingHandle.Fatalf("compose configuration: %s: %v", output, operationError)
	}
	var configuration struct {
		Services map[string]struct {
			Build       struct{ Context string }
			Environment map[string]string
			Restart     string
			Volumes     []struct{ Source, Target string }
			Command     []string
		}
		Networks map[string]struct{ IPAM struct{ Config []any } }
	}
	testutil.RequireNoError(testingHandle, json.Unmarshal(output, &configuration))
	if configuration.Services["ui"].Build.Context != filepath.Join(root, "ui") || configuration.Services["ui"].Environment["HARNESS_UPSTREAM"] != "http://mtt:8080" {
		testingHandle.Fatal("integrated UI context or upstream differs from its own image contract")
	}
	for _, name := range []string{"postgres", "mtt", "ui"} {
		if configuration.Services[name].Restart != "unless-stopped" {
			testingHandle.Fatalf("%s does not resume after Docker restarts", name)
		}
	}
	if len(configuration.Networks["default"].IPAM.Config) > 0 {
		testingHandle.Fatal("the portable stack requires a fixed subnet")
	}
	for _, volume := range configuration.Services["mtt"].Volumes {
		if volume.Target == "/host" {
			testingHandle.Fatal("host-root mount is enabled by default")
		}
	}
}
