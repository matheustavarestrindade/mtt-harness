package scripts_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

// This opt-in test builds the real images and runs start.sh from a fresh checkout
// fixture. Its unique Compose project and random loopback ports never touch the
// developer's live services or database volume.
func TestDockerStartLifecycle(testingHandle *testing.T) {
	if os.Getenv("MTT_TEST_START_DOCKER") != "1" {
		testingHandle.Skip("set MTT_TEST_START_DOCKER=1 to build and test the full Docker stack")
	}
	_, sourceFile, _, _ := runtime.Caller(0)
	root := filepath.Dir(filepath.Dir(sourceFile))
	operationContext, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	project := "mtt-start-test-" + fmt.Sprint(time.Now().UnixNano())
	environment := []string{}
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "MTT_") || strings.HasPrefix(name, "UI_") || strings.HasPrefix(name, "COMPOSE_") {
			continue
		}
		environment = append(environment, entry)
	}
	execute := func(directory string, extra []string, name string, arguments ...string) []byte {
		testingHandle.Helper()
		command := exec.CommandContext(operationContext, name, arguments...)
		command.Dir = directory
		command.Env = append(append([]string{}, environment...), extra...)
		output, operationError := command.CombinedOutput()
		if operationError != nil {
			testingHandle.Fatalf("%s %v failed: %v\n%s", name, arguments, operationError, output)
		}
		return output
	}
	testingHandle.Log("Building isolated images (first build downloads verified model assets)")
	execute(root, nil, "docker", "compose", "--env-file", os.DevNull, "-p", project, "build", "postgres", "mtt", "ui")
	temporary := testingHandle.TempDir()
	checkout := filepath.Join(temporary, "fresh checkout with spaces")
	testutil.RequireNoError(testingHandle, os.MkdirAll(checkout, 0700))
	files := []string{"start.sh", "compose.yaml", "mtt.example.json", "mcp.example.json", ".env.example", "providers.json", "start_prompt.md", "ui/compose.yaml", "docker/compose.host-files.yaml", "plugins/sidekick/prompt.md", "plugins/spaced_repetition/prompts/low.md", "plugins/spaced_repetition/prompts/medium.md", "plugins/spaced_repetition/prompts/high.md"}
	for _, name := range files {
		data, operationError := os.ReadFile(filepath.Join(root, name))
		testutil.RequireNoError(testingHandle, operationError)
		destination := filepath.Join(checkout, name)
		testutil.RequireNoError(testingHandle, os.MkdirAll(filepath.Dir(destination), 0700))
		testutil.RequireNoError(testingHandle, os.WriteFile(destination, data, 0600))
	}
	deployment := fmt.Sprintf("COMPOSE_PROJECT_NAME=%s\nMTT_HOST_PORT=127.0.0.1:0\nUI_BIND_ADDRESS=127.0.0.1\nUI_PORT=0\nMTT_TEST_PROVIDER=true\n", project)
	testutil.RequireNoError(testingHandle, os.WriteFile(filepath.Join(checkout, ".env.example"), []byte(deployment), 0600))
	compose := []string{"compose", "--project-directory", checkout, "--env-file", filepath.Join(checkout, ".env"), "-p", project, "-f", filepath.Join(checkout, "compose.yaml")}
	testingHandle.Cleanup(func() {
		cleanupContext, stop := context.WithTimeout(context.Background(), 2*time.Minute)
		defer stop()
		command := exec.CommandContext(cleanupContext, "docker", append(compose, "down", "--volumes", "--remove-orphans")...)
		command.Env = environment
		if output, operationError := command.CombinedOutput(); operationError != nil {
			testingHandle.Errorf("isolated stack cleanup failed: %v\n%s", operationError, output)
		}
	})
	testingHandle.Log("Starting a fresh fixture through start.sh")
	output := execute(temporary, nil, "bash", filepath.Join(checkout, "start.sh"), "--no-build")
	tokenMatch := regexp.MustCompile(`mtt API token: ([a-zA-Z0-9_-]+)`).FindSubmatch(output)
	if len(tokenMatch) != 2 {
		testingHandle.Fatalf("first startup did not expose its initial token: %s", output)
	}
	token := string(tokenMatch[1])
	endpoint := func(service string) string {
		address := strings.TrimSpace(string(execute(checkout, nil, "docker", append(compose, "port", service, "8080")...)))
		return "http://" + strings.Split(address, "\n")[0]
	}
	apiURL, uiURL := endpoint("mtt"), endpoint("ui")
	client := &http.Client{Timeout: 20 * time.Second}
	requestJSON := func(method, address string, body any, result any) {
		testingHandle.Helper()
		var encoded []byte
		if body != nil {
			var operationError error
			encoded, operationError = json.Marshal(body)
			testutil.RequireNoError(testingHandle, operationError)
		}
		request, operationError := http.NewRequestWithContext(operationContext, method, address, bytes.NewReader(encoded))
		testutil.RequireNoError(testingHandle, operationError)
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		response, operationError := client.Do(request)
		testutil.RequireNoError(testingHandle, operationError)
		defer response.Body.Close()
		data, operationError := io.ReadAll(response.Body)
		testutil.RequireNoError(testingHandle, operationError)
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			testingHandle.Fatalf("%s: HTTP %d: %s", address, response.StatusCode, data)
		}
		if result != nil {
			testutil.RequireNoError(testingHandle, json.Unmarshal(data, result))
		}
	}
	requestJSON("GET", apiURL+"/health", nil, nil)
	requestJSON("GET", uiURL+"/api/health", nil, nil)
	response, operationError := client.Get(uiURL)
	testutil.RequireNoError(testingHandle, operationError)
	html, operationError := io.ReadAll(response.Body)
	response.Body.Close()
	testutil.RequireNoError(testingHandle, operationError)
	if response.StatusCode != 200 || !strings.Contains(string(html), "/assets/") {
		testingHandle.Fatal("UI did not serve its built page")
	}
	projectDirectory := filepath.Join(checkout, "workspace", "project")
	testutil.RequireNoError(testingHandle, os.MkdirAll(projectDirectory, 0700))
	execute(checkout, nil, "docker", append(compose, "exec", "-T", "mtt", "sh", "-c", "printf host-write > /workspace/project/from-container.txt")...)
	data, operationError := os.ReadFile(filepath.Join(projectDirectory, "from-container.txt"))
	testutil.RequireNoError(testingHandle, operationError)
	if string(data) != "host-write" {
		testingHandle.Fatal("container write was not visible on the host")
	}
	var workspace struct{ ID string }
	requestJSON("POST", uiURL+"/api/instances", map[string]any{"workspace": "/workspace/project", "default_model": "test/test-model"}, &workspace)
	var session struct{ ID string }
	requestJSON("POST", uiURL+"/api/instances/"+workspace.ID+"/sessions", map[string]any{"model": "test/test-model"}, &session)
	requestJSON("PUT", uiURL+"/api/settings/agent_depth_limit", map[string]string{"value": "4"}, nil)
	requestJSON("POST", uiURL+"/api/sessions/"+session.ID+"/messages", map[string]string{"content": "Verify the portable stack."}, nil)
	deadline := time.Now().Add(20 * time.Second)
	answered := false
	for time.Now().Before(deadline) {
		var messages []struct{ Role string }
		requestJSON("GET", uiURL+"/api/sessions/"+session.ID+"/messages", nil, &messages)
		for _, message := range messages {
			if message.Role == "assistant" {
				answered = true
			}
		}
		if answered {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !answered {
		testingHandle.Fatal("test-provider conversation did not finish")
	}
	socketContext, stopSocket := context.WithTimeout(operationContext, 10*time.Second)
	defer stopSocket()
	socket, _, operationError := websocket.Dial(socketContext, strings.Replace(uiURL, "http://", "ws://", 1)+"/api/sessions/"+session.ID+"/events?view=true&token="+token, nil)
	testutil.RequireNoError(testingHandle, operationError)
	_, frame, operationError := socket.Read(socketContext)
	testutil.RequireNoError(testingHandle, operationError)
	socket.Close(websocket.StatusNormalClosure, "")
	if !bytes.Contains(frame, []byte(`"type":"state"`)) {
		testingHandle.Fatalf("UI proxy did not deliver the session snapshot: %s", frame)
	}
	testingHandle.Log("First start: API, UI, WebSocket, host writes and a real queued test-provider request passed")
	output = execute(temporary, nil, "bash", filepath.Join(checkout, "start.sh"), "--no-build")
	if bytes.Contains(output, []byte("mtt API token:")) {
		testingHandle.Fatal("repeat startup reprinted the first-start token")
	}
	execute(checkout, nil, "docker", append(compose, "stop", "mtt", "ui", "postgres")...)
	execute(temporary, nil, "bash", filepath.Join(checkout, "start.sh"), "--no-build")
	uiURL = endpoint("ui")
	var saved struct{ ID string }
	requestJSON("GET", uiURL+"/api/sessions/"+session.ID, nil, &saved)
	if saved.ID != session.ID {
		testingHandle.Fatal("session was not retained across stop/start")
	}
	var settings map[string]string
	requestJSON("GET", uiURL+"/api/settings", nil, &settings)
	if settings["agent_depth_limit"] != "4" {
		testingHandle.Fatal("database settings changed across restart")
	}
	hostDirectory := filepath.Join(temporary, "host files with spaces")
	testutil.RequireNoError(testingHandle, os.MkdirAll(hostDirectory, 0700))
	testutil.RequireNoError(testingHandle, os.WriteFile(filepath.Join(hostDirectory, "marker.txt"), []byte("host-root-check"), 0600))
	execute(temporary, []string{"MTT_HOST_ROOT=" + hostDirectory}, "bash", filepath.Join(checkout, "start.sh"), "--no-build")
	execute(checkout, nil, "docker", append(compose, "exec", "-T", "mtt", "sh", "-c", "test -f /host/marker.txt && printf host-extra-write > /host/from-container.txt")...)
	if _, operationError := os.Stat(filepath.Join(hostDirectory, "from-container.txt")); operationError != nil {
		testingHandle.Fatal("optional /host mount did not reach the host directory")
	}
	testingHandle.Log("Repeated start, stop/start persistence, token retention and optional host mount passed")
}
