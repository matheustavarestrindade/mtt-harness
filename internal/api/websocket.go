package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (s *Server) sessionEvents(w http.ResponseWriter, r *http.Request) {
	sessionID := atom.SessionID(r.PathValue("id"))
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	ctx := conn.CloseRead(r.Context())
	writer := newWriter(conn)
	unsubscribe := s.bus.On("*", func(ctx context.Context, event atom.Event) {
		if event.SessionID != sessionID {
			return
		}
		writer.send(event)
	})
	defer unsubscribe()
	<-ctx.Done()
}

func (s *Server) processOutput(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	stdout, stderr, ok := s.processes.Output(id)
	if !ok {
		http.Error(w, "the process is not in the manager", http.StatusNotFound)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	ctx := conn.CloseRead(r.Context())
	writer := newWriter(conn)
	writer.send(map[string]any{"stream": "stdout", "data": string(stdout)})
	writer.send(map[string]any{"stream": "stderr", "data": string(stderr)})
	events, ok := s.processes.Subscribe(id)
	if !ok {
		return
	}
	for event := range events {
		writer.send(map[string]any{"stream": event.Stream, "data": string(event.Data), "error": event.Error})
	}
	<-ctx.Done()
}

type writer struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func newWriter(conn *websocket.Conn) *writer {
	return &writer{conn: conn}
}

func (w *writer) send(value any) {
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = w.conn.Write(ctx, websocket.MessageText, data)
}
