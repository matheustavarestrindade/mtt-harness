package scripts_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestNativeBuildAndLaunch(testingHandle *testing.T) {
	if os.Getenv("MTT_TEST_NATIVE_LAUNCHER") != "1" {
		testingHandle.Skip("set MTT_TEST_NATIVE_LAUNCHER=1 for the real host/Docker check")
	}
	if runtime.GOOS != "linux" {
		testingHandle.Skip("native embedding libraries currently target Linux")
	}
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Dir(filepath.Dir(source))
	operationContext, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	environment := []string{}
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		if name == "PATH" || strings.HasPrefix(name, "MTT_") || strings.HasPrefix(name, "UI_") || strings.HasPrefix(name, "COMPOSE_") {
			continue
		}
		environment = append(environment, value)
	}
	environment = append(environment, "NATIVE_HOST_PROBE=host-environment-retained")
	execute := func(directory string, additions []string, arguments ...string) []byte {
		testingHandle.Helper()
		command := exec.CommandContext(operationContext, arguments[0], arguments[1:]...)
		command.Dir, command.Env = directory, append(append([]string{}, environment...), additions...)
		output, operationError := command.CombinedOutput()
		if operationError != nil {
			testingHandle.Fatalf("native command failed: %v\n%s", operationError, output)
		}
		return output
	}
	hostPath := "PATH=" + os.Getenv("PATH")
	testingHandle.Log("Compiling the full host binary and UI through build.sh")
	execute(root, []string{hostPath}, "bash", "build.sh", "--build-only")
	manifestBytes, operationError := os.ReadFile(filepath.Join(root, "bin/native/build.json"))
	testutil.RequireNoError(testingHandle, operationError)
	temporary := testingHandle.TempDir()
	checkout := filepath.Join(temporary, "native checkout with spaces")
	for _, name := range []string{"build.sh", "compose.yaml", "mtt.example.json", "mcp.example.json", ".env.example", "providers.json", "start_prompt.md", "docker/postgres.Dockerfile", "docker/compose.native.yaml", "ui/compose.yaml", "ui/vite.config.ts", "scripts/native.py", "scripts/native_build.py", "scripts/native_common.py", "scripts/native_runtime.py", "plugins/sidekick/prompt.md", "plugins/spaced_repetition/prompts/low.md", "plugins/spaced_repetition/prompts/medium.md", "plugins/spaced_repetition/prompts/high.md"} {
		data, readError := os.ReadFile(filepath.Join(root, name))
		testutil.RequireNoError(testingHandle, readError)
		destination := filepath.Join(checkout, name)
		testutil.RequireNoError(testingHandle, os.MkdirAll(filepath.Dir(destination), 0700))
		testutil.RequireNoError(testingHandle, os.WriteFile(destination, data, 0600))
	}
	testutil.RequireNoError(testingHandle, os.Symlink(filepath.Join(root, "ui/node_modules"), filepath.Join(checkout, "ui/node_modules")))
	stateDirectory := filepath.Join(checkout, "bin/native")
	testutil.RequireNoError(testingHandle, os.MkdirAll(stateDirectory, 0700))
	testutil.RequireNoError(testingHandle, os.WriteFile(filepath.Join(stateDirectory, "build.json"), manifestBytes, 0600))
	probeDirectory := filepath.Join(temporary, "host programs")
	testutil.RequireNoError(testingHandle, os.MkdirAll(probeDirectory, 0700))
	testutil.RequireNoError(testingHandle, os.WriteFile(filepath.Join(probeDirectory, "host-tool-probe"), []byte("#!/bin/sh\nprintf 'HOST_TOOL_OK\\n'\n"), 0700))
	hostPath = "PATH=" + probeDirectory + string(os.PathListSeparator) + os.Getenv("PATH")
	project := fmt.Sprintf("mtt-native-test-%d", time.Now().UnixNano())
	freePort := func() int {
		listener, listenError := net.Listen("tcp", "127.0.0.1:0")
		testutil.RequireNoError(testingHandle, listenError)
		port := listener.Addr().(*net.TCPAddr).Port
		listener.Close()
		return port
	}
	apiPort, uiPort := freePort(), freePort()
	for apiPort == uiPort {
		uiPort = freePort()
	}
	deployment := fmt.Sprintf("COMPOSE_PROJECT_NAME=%s\nMTT_HOST_PORT=%d\nUI_PORT=%d\nUI_BIND_ADDRESS=127.0.0.1\nMTT_POSTGRES_PORT=0\nMTT_TEST_PROVIDER=false\n", project, apiPort, uiPort)
	testutil.RequireNoError(testingHandle, os.WriteFile(filepath.Join(checkout, ".env.example"), []byte(deployment), 0600))
	var token string
	bootstrap := map[string]any{"api_token": "", "port": 8080, "providers_file": "providers.json", "mcp_file": "mcp.json"}
	encoded, operationError := json.Marshal(bootstrap)
	testutil.RequireNoError(testingHandle, operationError)
	testutil.RequireNoError(testingHandle, os.WriteFile(filepath.Join(checkout, "mtt.example.json"), encoded, 0600))
	provider := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		var input struct {
			Messages []struct {
				Role       string `json:"role"`
				ToolCallID string `json:"tool_call_id"`
			}
			Tools []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			}
		}
		if decodeError := json.NewDecoder(request.Body).Decode(&input); decodeError != nil {
			http.Error(responseWriter, decodeError.Error(), 400)
			return
		}
		hasBash, finished := false, false
		for _, tool := range input.Tools {
			hasBash = hasBash || tool.Function.Name == "bash"
		}
		for _, message := range input.Messages {
			finished = finished || message.ToolCallID == "run-host-command"
		}
		responseWriter.Header().Set("Content-Type", "text/event-stream")
		delta := map[string]any{"content": "Native verification complete."}
		reason := "stop"
		if !finished {
			name, identifier, arguments := "search_tool", "discover-shell", `{"query":"bash"}`
			if hasBash {
				name, identifier = "bash", "run-host-command"
				data, _ := json.Marshal(map[string]any{"command": "host-tool-probe && python3 -c 'import os; print(\"HOST_UID\", os.getuid()); open(\"native-write.txt\", \"w\").write(\"host write\")'", "wait": true, "terminal": false, "idle_timeout": 0})
				arguments = string(data)
			}
			delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": identifier, "type": "function", "function": map[string]any{"name": name, "arguments": arguments}}}}
			reason = "tool_calls"
		}
		data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": reason}}, "usage": map[string]int{"prompt_tokens": 20, "completion_tokens": 5}})
		fmt.Fprintf(responseWriter, "data: %s\n\ndata: [DONE]\n\n", data)
	}))
	defer provider.Close()
	var providers map[string]any
	providerBytes, operationError := os.ReadFile(filepath.Join(checkout, "providers.json"))
	testutil.RequireNoError(testingHandle, operationError)
	testutil.RequireNoError(testingHandle, json.Unmarshal(providerBytes, &providers))
	providers["providers"] = []any{map[string]any{"name": "synthetic", "protocol": "chat_completions", "authentication": "none", "api_url": provider.URL, "models": []any{map[string]any{"id": "native-model", "context_max": 65536, "output_max": 1024, "tools": true, "input": []string{"text"}}}}}
	providerBytes, operationError = json.Marshal(providers)
	testutil.RequireNoError(testingHandle, operationError)
	testutil.RequireNoError(testingHandle, os.WriteFile(filepath.Join(checkout, "providers.json"), providerBytes, 0600))
	compose := []string{"compose", "--project-directory", checkout, "--env-file", filepath.Join(checkout, ".env"), "-p", project, "-f", filepath.Join(checkout, "compose.yaml"), "-f", filepath.Join(checkout, "docker/compose.native.yaml")}
	var foreground *exec.Cmd
	stopForeground := func() {
		if foreground != nil {
			_ = foreground.Process.Signal(os.Interrupt)
			_ = foreground.Wait()
			foreground = nil
		}
	}
	testingHandle.Cleanup(func() {
		stopForeground()
		cleanupContext, stop := context.WithTimeout(context.Background(), 2*time.Minute)
		defer stop()
		for _, arguments := range [][]string{{"bash", filepath.Join(checkout, "build.sh"), "--stop"}, append([]string{"docker"}, append(compose, "down", "--volumes", "--remove-orphans")...)} {
			cleanup := exec.CommandContext(cleanupContext, arguments[0], arguments[1:]...)
			cleanup.Env = append(append([]string{}, environment...), hostPath)
			cleanup.Dir = temporary
			if output, cleanupError := cleanup.CombinedOutput(); cleanupError != nil {
				testingHandle.Errorf("native fixture cleanup: %v\n%s", cleanupError, output)
			}
		}
	})
	startForeground := func() {
		logFile, createError := os.Create(filepath.Join(temporary, "launcher.log"))
		testutil.RequireNoError(testingHandle, createError)
		foreground = exec.CommandContext(operationContext, "bash", filepath.Join(checkout, "build.sh"), "--no-build", "--foreground")
		foreground.Dir, foreground.Env = temporary, append(append([]string{}, environment...), hostPath)
		foreground.Stdout, foreground.Stderr = logFile, logFile
		testutil.RequireNoError(testingHandle, foreground.Start())
		logFile.Close()
	}
	client := &http.Client{Timeout: 15 * time.Second}
	apiURL, uiURL := fmt.Sprintf("http://127.0.0.1:%d", apiPort), fmt.Sprintf("http://127.0.0.1:%d", uiPort)
	waitReady := func() {
		deadline := time.Now().Add(180 * time.Second)
		for time.Now().Before(deadline) {
			response, requestError := client.Get(uiURL + "/api/health")
			if requestError == nil {
				response.Body.Close()
				if response.StatusCode == 200 {
					return
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
		for _, path := range []string{filepath.Join(temporary, "launcher.log"), filepath.Join(stateDirectory, "api.log"), filepath.Join(stateDirectory, "ui.log")} {
			data, _ := os.ReadFile(path)
			testingHandle.Logf("%s: %s", filepath.Base(path), data)
		}
		testingHandle.Fatal("native API/UI did not become ready")
	}
	waitForegroundOutput := func() []byte {
		testingHandle.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			output, readError := os.ReadFile(filepath.Join(temporary, "launcher.log"))
			testutil.RequireNoError(testingHandle, readError)
			if bytes.Contains(output, []byte("Running in this terminal.")) {
				return output
			}
			time.Sleep(50 * time.Millisecond)
		}
		testingHandle.Fatal("native launcher did not finish its startup summary")
		return nil
	}
	requestJSON := func(method, path string, body, result any) {
		testingHandle.Helper()
		var payload []byte
		if body != nil {
			payload, operationError = json.Marshal(body)
			testutil.RequireNoError(testingHandle, operationError)
		}
		request, requestError := http.NewRequestWithContext(operationContext, method, uiURL+"/api"+path, bytes.NewReader(payload))
		testutil.RequireNoError(testingHandle, requestError)
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		response, requestError := client.Do(request)
		testutil.RequireNoError(testingHandle, requestError)
		defer response.Body.Close()
		data, readError := io.ReadAll(response.Body)
		testutil.RequireNoError(testingHandle, readError)
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			testingHandle.Fatalf("%s returned %d: %s", path, response.StatusCode, data)
		}
		if result != nil {
			testutil.RequireNoError(testingHandle, json.Unmarshal(data, result))
		}
	}
	startForeground()
	waitReady()
	firstOutput := waitForegroundOutput()
	tokenMatch := regexp.MustCompile(`(?m)^API token: ([0-9a-f]{64})$`).FindSubmatch(firstOutput)
	if len(tokenMatch) != 2 {
		testingHandle.Fatal("first native launch did not display its generated API token")
	}
	token = string(tokenMatch[1])
	if bytes.Count(firstOutput, []byte(token)) != 1 || !bytes.Contains(firstOutput, []byte("UI API address: /api")) {
		testingHandle.Fatal("first-start credentials were missing or duplicated in log output")
	}
	stateBytes, readError := os.ReadFile(filepath.Join(stateDirectory, "runtime.json"))
	testutil.RequireNoError(testingHandle, readError)
	var launchState map[string]any
	testutil.RequireNoError(testingHandle, json.Unmarshal(stateBytes, &launchState))
	if launchState["api_token_displayed"] != true || bytes.Contains(stateBytes, []byte(token)) {
		testingHandle.Fatal("launch state did not record display, or stored a duplicate credential")
	}
	runningServices := strings.TrimSpace(string(execute(checkout, []string{hostPath}, append([]string{"docker"}, append(compose, "ps", "--services", "--status", "running")...)...)))
	if runningServices != "postgres" {
		testingHandle.Fatalf("unexpected Docker services in native mode: %s", runningServices)
	}
	var contextSettings struct{ Available bool }
	requestJSON("GET", "/plugins/context/settings", nil, &contextSettings)
	if !contextSettings.Available {
		testingHandle.Fatal("native workspace-memory encoder is unavailable")
	}
	workspaceDirectory := filepath.Join(temporary, "host project")
	testutil.RequireNoError(testingHandle, os.MkdirAll(workspaceDirectory, 0700))
	var workspace, session struct{ ID string }
	requestJSON("POST", "/instances", map[string]string{"workspace": workspaceDirectory, "default_model": "synthetic/native-model"}, &workspace)
	requestJSON("POST", "/instances/"+workspace.ID+"/sessions", map[string]string{"model": "synthetic/native-model"}, &session)
	requestJSON("PUT", "/settings/agent_depth_limit", map[string]string{"value": "4"}, nil)
	requestJSON("POST", "/sessions/"+session.ID+"/messages", map[string]string{"content": "Verify host command access."}, nil)
	deadline := time.Now().Add(40 * time.Second)
	verified := false
	for time.Now().Before(deadline) {
		var state struct{ Permissions []struct{ ID string } }
		requestJSON("GET", "/sessions/"+session.ID+"/state", nil, &state)
		for _, permission := range state.Permissions {
			requestJSON("POST", "/permissions/"+permission.ID, map[string]string{"kind": "allow", "scope": "once"}, nil)
		}
		var messages []atom.Message
		requestJSON("GET", "/sessions/"+session.ID+"/messages", nil, &messages)
		for _, message := range messages {
			for _, part := range message.Content {
				if message.Role == atom.RoleTool && strings.Contains(part.Text, "HOST_TOOL_OK") && strings.Contains(part.Text, fmt.Sprintf("HOST_UID %d", os.Getuid())) {
					verified = true
				}
			}
		}
		if verified {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !verified {
		testingHandle.Fatal("the harness did not execute the host PATH probe and Python with the host UID")
	}
	idle := false
	for time.Now().Before(deadline) {
		var status struct {
			Running bool
			Error   string
		}
		requestJSON("GET", "/sessions/"+session.ID+"/status", nil, &status)
		if status.Error != "" {
			testingHandle.Fatal(status.Error)
		}
		if !status.Running {
			idle = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !idle {
		testingHandle.Fatal("native tool turn did not become idle")
	}
	data, operationError := os.ReadFile(filepath.Join(workspaceDirectory, "native-write.txt"))
	testutil.RequireNoError(testingHandle, operationError)
	if string(data) != "host write" {
		testingHandle.Fatal("native tool write did not reach the host workspace")
	}
	socketContext, stopSocket := context.WithTimeout(operationContext, 10*time.Second)
	defer stopSocket()
	socket, _, operationError := websocket.Dial(socketContext, strings.Replace(uiURL, "http://", "ws://", 1)+"/api/sessions/"+session.ID+"/events?view=true&token="+token, nil)
	testutil.RequireNoError(testingHandle, operationError)
	_, frame, operationError := socket.Read(socketContext)
	testutil.RequireNoError(testingHandle, operationError)
	socket.Close(websocket.StatusNormalClosure, "")
	if !bytes.Contains(frame, []byte(`"type":"state"`)) {
		testingHandle.Fatal("native UI did not proxy the session WebSocket")
	}
	testingHandle.Log("Native API/UI, embeddings, host PATH/Python/UID, host files and WebSocket passed")
	stopForeground()
	execute(checkout, []string{hostPath}, append([]string{"docker"}, append(compose, "stop", "postgres")...)...)
	// An existing installation from before this feature has no display marker.
	// A configured token is also not printed by the harness's generation logger.
	delete(launchState, "api_token_displayed")
	stateBytes, operationError = json.Marshal(launchState)
	testutil.RequireNoError(testingHandle, operationError)
	testutil.RequireNoError(testingHandle, os.WriteFile(filepath.Join(stateDirectory, "runtime.json"), stateBytes, 0600))
	bootstrap["api_token"] = token
	encoded, operationError = json.Marshal(bootstrap)
	testutil.RequireNoError(testingHandle, operationError)
	testutil.RequireNoError(testingHandle, os.WriteFile(filepath.Join(checkout, "mtt.json"), encoded, 0600))
	serviceAvailable := exec.Command("systemctl", "--user", "show-environment").Run() == nil
	var upgradeOutput []byte
	if serviceAvailable {
		upgradeOutput = execute(temporary, []string{hostPath}, "bash", filepath.Join(checkout, "build.sh"), "--no-build")
	} else {
		startForeground()
	}
	waitReady()
	if !serviceAvailable {
		upgradeOutput = waitForegroundOutput()
	}
	if bytes.Count(upgradeOutput, []byte(token)) != 1 || !bytes.Contains(upgradeOutput, []byte("API token: "+token)) {
		testingHandle.Fatal("legacy native state did not display the existing configured token once")
	}
	if serviceAvailable {
		var runtimeState struct{ Units map[string]string }
		stateBytes, readError := os.ReadFile(filepath.Join(stateDirectory, "runtime.json"))
		testutil.RequireNoError(testingHandle, readError)
		testutil.RequireNoError(testingHandle, json.Unmarshal(stateBytes, &runtimeState))
		serviceEnvironment := execute(checkout, []string{hostPath}, "systemctl", "--user", "show", runtimeState.Units["api"], "--property=Environment", "--value")
		if !bytes.Contains(serviceEnvironment, []byte("NATIVE_HOST_PROBE=host-environment-retained")) {
			testingHandle.Fatal("user service did not inherit the host environment")
		}
	}
	var saved struct{ ID string }
	requestJSON("GET", "/sessions/"+session.ID, nil, &saved)
	var settings map[string]string
	requestJSON("GET", "/settings", nil, &settings)
	if saved.ID != session.ID || settings["agent_depth_limit"] != "4" {
		testingHandle.Fatal("native restart changed saved data")
	}
	var repeatedOutput []byte
	if serviceAvailable {
		repeatedOutput = execute(temporary, []string{hostPath}, "bash", filepath.Join(checkout, "build.sh"), "--no-build", "--restart")
	} else {
		stopForeground()
		startForeground()
	}
	waitReady()
	if !serviceAvailable {
		repeatedOutput = waitForegroundOutput()
	}
	if bytes.Contains(repeatedOutput, []byte(token)) || bytes.Contains(repeatedOutput, []byte("API token:")) {
		testingHandle.Fatal("repeat native startup displayed the token again")
	}
	requestJSON("GET", "/sessions/"+session.ID, nil, &saved)
	response, operationError := client.Get(apiURL + "/health")
	testutil.RequireNoError(testingHandle, operationError)
	response.Body.Close()
	testingHandle.Log("First-launch token, legacy/configured-token display, repeat suppression, and restart persistence passed")
}
