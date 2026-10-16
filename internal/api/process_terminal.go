package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/process"
)

type terminalControl struct {
	Type       string  `json:"type"`
	Data       string  `json:"data"`
	DataBase64 *string `json:"data_base64"`
	Columns    int     `json:"columns"`
	Rows       int     `json:"rows"`
	Signal     string  `json:"signal"`
}

func (server *Server) processOutput(responseWriter http.ResponseWriter, request *http.Request) {
	identifier := request.PathValue("id")
	if server.processes == nil {
		writeError(responseWriter, http.StatusServiceUnavailable, "process service is unavailable")
		return
	}
	running, found := server.processes.Get(identifier)
	if !found {
		writeError(responseWriter, http.StatusNotFound, "process output is not retained")
		return
	}
	interactive, _ := running.(harness.InteractiveProcess)
	connection, operationError := websocket.Accept(responseWriter, request, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
	if operationError != nil {
		return
	}
	connection.SetReadLimit(16 * 1024)
	operationContext, cancel := context.WithCancel(request.Context())
	controls := make(chan error, 1)
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		defer cancel()
		server.readTerminalControls(operationContext, connection, interactive, controls)
	}()
	defer func() { cancel(); _ = connection.CloseNow(); <-readerDone }()
	events, found := server.processes.SubscribeContext(operationContext, identifier)
	if !found {
		return
	}
	for {
		select {
		case <-operationContext.Done():
			return
		case controlError := <-controls:
			if sendJSON(operationContext, connection, map[string]any{"process_id": identifier, "stream": "control", "error": controlError.Error()}) != nil {
				return
			}
		case event, open := <-events:
			if !open {
				return
			}
			frame := map[string]any{"process_id": event.ProcessID, "stream": event.Stream, "data": string(event.Data), "data_base64": event.Data, "offset": event.Offset, "snapshot": event.Snapshot, "at": event.At, "error": event.Error, "exit": event.Exit}
			if event.Stream == atom.StreamStart {
				frame["terminal"] = interactive != nil && interactive.HasTerminal()
				frame["input"] = interactive != nil && interactive.Running()
			}
			if sendJSON(operationContext, connection, frame) != nil {
				return
			}
			if event.Stream == atom.StreamExit {
				_ = connection.Close(websocket.StatusNormalClosure, "")
				return
			}
		}
	}
}

func (server *Server) readTerminalControls(operationContext context.Context, connection *websocket.Conn, running harness.InteractiveProcess, failures chan<- error) {
	for {
		kind, data, operationError := connection.Read(operationContext)
		if operationError != nil {
			return
		}
		var control terminalControl
		if kind != websocket.MessageText {
			operationError = fmt.Errorf("terminal controls require JSON text")
		} else {
			operationError = json.Unmarshal(data, &control)
		}
		if operationError == nil {
			operationError = applyTerminalControl(operationContext, running, control)
		}
		if operationError != nil {
			select {
			case failures <- operationError:
			case <-operationContext.Done():
				return
			}
		}
	}
}

func applyTerminalControl(operationContext context.Context, running harness.InteractiveProcess, control terminalControl) error {
	if running == nil || !running.Running() {
		return fmt.Errorf("process input is not available")
	}
	switch control.Type {
	case "input":
		data := []byte(control.Data)
		if control.DataBase64 != nil {
			var operationError error
			data, operationError = base64.StdEncoding.DecodeString(*control.DataBase64)
			if operationError != nil {
				return fmt.Errorf("terminal input is not valid base64")
			}
		}
		if len(data) > process.MaximumInputBytes {
			return fmt.Errorf("process input exceeds %d bytes", process.MaximumInputBytes)
		}
		writeContext, cancel := context.WithTimeout(operationContext, 2*time.Second)
		defer cancel()
		return running.WriteContext(writeContext, data)
	case "resize":
		return running.ResizeTerminal(control.Columns, control.Rows)
	case "signal":
		if control.Signal != "interrupt" {
			return fmt.Errorf("terminal signal must be interrupt")
		}
		return running.Kill("interrupt")
	default:
		return fmt.Errorf("unknown terminal control type %q", control.Type)
	}
}
