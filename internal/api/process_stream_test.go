package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/api"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/eventbus"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/process"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/processes"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestProcessStreamPreservesSplitUTF8OffsetsExitAndToolOrigin(test *testing.T) {
	operationContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	database := memory.New()
	harnessRuntime := harness.New()
	bus := eventbus.New(harnessRuntime)
	processManager := processes.New(process.New(2), database, harnessRuntime, bus, nil)
	test.Cleanup(func() { testutil.RequireNoError(test, processManager.Close(context.Background())) })
	session := atom.Session{ID: "stream-session", InstanceID: "instance"}
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	server := httptest.NewServer(api.New(api.Config{Token: "secret", Store: database, Bus: bus, Processes: processManager}).Handler())
	defer server.Close()
	processContext := harness.WithToolOrigin(harness.WithSession(operationContext, session), harness.ToolOrigin{MessageID: "origin-message", CallID: "origin-call"})
	runningProcess, operationError := processManager.Start(processContext, atom.ProcessSpec{Command: "sh", Args: []string{"-c", `printf '\033[36m\342'; read reply; printf '\202\254\033[0m\n'; exit 7`}, Notify: atom.NotifyPolicy{Mode: atom.NotifyNone}})
	testutil.RequireNoError(test, operationError)
	streamURL := strings.Replace(server.URL, "http://", "ws://", 1) + "/processes/" + runningProcess.ID() + "/output?token=secret"
	connect := func() *websocket.Conn {
		connection, _, operationError := websocket.Dial(operationContext, streamURL, nil)
		testutil.RequireNoError(test, operationError)
		connection.SetReadLimit(1024 * 1024)
		return connection
	}
	type outputFrame struct {
		ProcessID string           `json:"process_id"`
		Stream    string           `json:"stream"`
		Data      string           `json:"data"`
		Bytes     []byte           `json:"data_base64"`
		Offset    int64            `json:"offset"`
		Snapshot  bool             `json:"snapshot"`
		Exit      *atom.ExitStatus `json:"exit"`
	}
	read := func(connection *websocket.Conn) outputFrame {
		_, data, operationError := connection.Read(operationContext)
		testutil.RequireNoError(test, operationError)
		var frame outputFrame
		testutil.RequireNoError(test, json.Unmarshal(data, &frame))
		if frame.ProcessID != runningProcess.ID() {
			test.Fatalf("wrong process frame: %+v", frame)
		}
		return frame
	}
	connection := connect()
	defer connection.CloseNow()
	var output []byte
	released := false
	for {
		frame := read(connection)
		if frame.Stream == "stdout" {
			if frame.Offset != int64(len(output)) {
				test.Fatalf("offset %d after %d bytes", frame.Offset, len(output))
			}
			output = append(output, frame.Bytes...)
		}
		if !released && len(output) >= 6 {
			released = true
			testutil.RequireNoError(test, runningProcess.Write([]byte("continue\n")))
		}
		if frame.Stream == "exit" {
			if frame.Exit == nil || frame.Exit.Code != 7 {
				test.Fatalf("lost exit status: %+v", frame)
			}
			break
		}
	}
	expected := []byte("\x1b[36m€\x1b[0m\n")
	if !bytes.Equal(output, expected) {
		test.Fatalf("wire changed UTF-8/ANSI bytes: %q", output)
	}
	reconnected := connect()
	defer reconnected.CloseNow()
	var replayed []byte
	for {
		frame := read(reconnected)
		if frame.Stream == "stdout" {
			if !frame.Snapshot {
				test.Fatal("retained output has no snapshot marker")
			}
			replayed = append(replayed, frame.Bytes...)
		}
		if frame.Stream == "exit" {
			break
		}
	}
	if !bytes.Equal(replayed, expected) {
		test.Fatalf("reconnect duplicated/lost output: %q", replayed)
	}
	var records []atom.ProcessRecord
	decode(test, request(test, server.URL+"/sessions/stream-session/processes", "secret", "GET", nil), &records)
	if len(records) != 1 || records[0].Spec.ToolMessageID != "origin-message" || records[0].Spec.ToolCallID != "origin-call" {
		test.Fatalf("public process origin: %+v", records)
	}
}
