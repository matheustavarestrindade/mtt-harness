package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/coder/websocket"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (server *Server) sessionEvents(responseWriter http.ResponseWriter, request *http.Request) {
	sessionID := atom.SessionID(request.PathValue("id"))
	session, operationError := server.store.Sessions().Get(request.Context(), sessionID)
	if respondToError(responseWriter, http.StatusNotFound, operationError) {
		return
	}
	var cursor uint64
	if value := request.URL.Query().Get("since"); value != "" {
		cursor, operationError = strconv.ParseUint(value, 10, 63)
		if respondToError(responseWriter, http.StatusBadRequest, operationError) {
			return
		}
	}
	connection, operationError := websocket.Accept(responseWriter, request, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
	if operationError != nil {
		return
	}
	defer connection.Close(websocket.StatusNormalClosure, "")
	operationContext := connection.CloseRead(request.Context())
	changed := make(chan struct{}, 1)
	unsubscribe := server.bus.On("*", func(_ context.Context, event atom.Event) {
		if event.SessionID != sessionID {
			return
		}
		select {
		case changed <- struct{}{}:
		default:
		}
	})
	defer unsubscribe()
	for {
		// Subscribe before replay to close the replay/live gap. A callback only
		// wakes this reader, so slow clients cannot block runtime publishers.
		events, operationError := server.store.Events().Since(operationContext, session.InstanceID, cursor)
		if operationError != nil {
			return
		}
		for _, event := range events {
			cursor = event.Seq
			if event.SessionID != sessionID {
				continue
			}
			if operationError := sendJSON(operationContext, connection, event); operationError != nil {
				return
			}
		}
		if len(events) == 1000 {
			continue
		}
		select {
		case <-operationContext.Done():
			return
		case <-changed:
		}
	}
}

func (server *Server) processOutput(responseWriter http.ResponseWriter, request *http.Request) {
	identifier := request.PathValue("id")
	if _, found := server.processes.Get(identifier); !found {
		writeError(responseWriter, http.StatusNotFound, "process output is not retained")
		return
	}
	connection, operationError := websocket.Accept(responseWriter, request, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
	if operationError != nil {
		return
	}
	defer connection.Close(websocket.StatusNormalClosure, "")
	operationContext := connection.CloseRead(request.Context())
	events, found := server.processes.SubscribeContext(operationContext, identifier)
	if !found {
		return
	}
	for {
		select {
		case <-operationContext.Done():
			return
		case event, open := <-events:
			if !open {
				return
			}
			if operationError := sendJSON(operationContext, connection, map[string]any{"stream": event.Stream, "data": string(event.Data), "error": event.Error}); operationError != nil {
				return
			}
			if event.Stream == atom.StreamExit {
				return
			}
		}
	}
}

func sendJSON(operationContext context.Context, connection *websocket.Conn, value any) error {
	data, operationError := json.Marshal(value)
	if operationError != nil {
		return operationError
	}
	writeContext, cancel := context.WithTimeout(operationContext, 5*time.Second)
	defer cancel()
	return connection.Write(writeContext, websocket.MessageText, data)
}
