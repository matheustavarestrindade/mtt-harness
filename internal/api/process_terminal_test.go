//go:build unix

package api_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/api"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/process"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/processes"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestProcessSocketAcceptsInputResizeAndControlCWithoutStoppingOnViewerClose(test *testing.T) {
	operationContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	database := memory.New()
	runtime := harness.New()
	manager := processes.New(process.New(4), database, runtime, nil, nil)
	defer manager.Close(context.Background())
	session := atom.Session{ID: "terminal-session", InstanceID: "terminal-workspace"}
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	running, operationError := manager.Start(harness.WithSession(operationContext, session), atom.ProcessSpec{Command: "sh", Args: []string{"-c", `trap 'exit 130' INT; printf '\033[32mREADY\033[0m\n'; IFS= read -r input; printf 'RECEIVED=%s\n' "$input"; stty size; while :; do sleep 0.05; done`}, Terminal: true, Notify: atom.NotifyPolicy{Mode: atom.NotifyNone}})
	testutil.RequireNoError(test, operationError)
	server := httptest.NewServer(api.New(api.Config{Token: "secret", Store: database, Processes: manager}).Handler())
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/processes/" + running.ID() + "/output?token=secret"
	connection, _, operationError := websocket.Dial(operationContext, url, nil)
	testutil.RequireNoError(test, operationError)
	var output strings.Builder
	readFrame := func(connection *websocket.Conn) map[string]any {
		test.Helper()
		_, data, operationError := connection.Read(operationContext)
		testutil.RequireNoError(test, operationError)
		var event map[string]any
		testutil.RequireNoError(test, json.Unmarshal(data, &event))
		if encoded, ok := event["data_base64"].(string); ok {
			bytes, operationError := base64.StdEncoding.DecodeString(encoded)
			testutil.RequireNoError(test, operationError)
			output.Write(bytes)
		}
		return event
	}
	start := readFrame(connection)
	if start["stream"] != "start" || start["terminal"] != true || start["input"] != true {
		test.Fatalf("terminal capability metadata: %#v", start)
	}
	for !strings.Contains(output.String(), "READY") {
		readFrame(connection)
	}
	testutil.RequireNoError(test, connection.Close(websocket.StatusNormalClosure, "viewer closed"))
	if !running.(*process.Handle).Running() {
		test.Fatal("viewer close stopped the process")
	}
	connection, _, operationError = websocket.Dial(operationContext, url, nil)
	testutil.RequireNoError(test, operationError)
	defer connection.CloseNow()
	readFrame(connection)
	send := func(value any) {
		test.Helper()
		data, operationError := json.Marshal(value)
		testutil.RequireNoError(test, operationError)
		testutil.RequireNoError(test, connection.Write(operationContext, websocket.MessageText, data))
	}
	send(map[string]any{"type": "resize", "columns": 97, "rows": 26})
	send(map[string]any{"type": "input", "data_base64": base64.StdEncoding.EncodeToString([]byte("Unicode 🌍\n"))})
	for !strings.Contains(output.String(), "RECEIVED=Unicode 🌍") || !strings.Contains(output.String(), "26 97") {
		readFrame(connection)
	}
	send(map[string]any{"type": "input", "data_base64": "Aw=="})
	for {
		event := readFrame(connection)
		if event["stream"] == "exit" {
			break
		}
	}
	_, _, operationError = connection.Read(operationContext)
	if websocket.CloseStatus(operationError) != websocket.StatusNormalClosure {
		test.Fatalf("socket did not close on process exit: %v", operationError)
	}
	if !strings.Contains(output.String(), "\x1b[32mREADY\x1b[0m") {
		test.Fatal("ANSI bytes were stripped from socket output")
	}
}
