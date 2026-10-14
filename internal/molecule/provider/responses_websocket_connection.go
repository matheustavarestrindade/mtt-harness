package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// A continuous reader processes control frames while a session is idle. It
// retains at most one event ahead of the decoder; cancellation closes the socket
// and joins the reader rather than leaving an abandoned model response behind.
type responsesWebsocketConnection struct {
	connection *websocket.Conn
	opened     time.Time
	events     chan []byte
	done       chan struct{}
	closed     atomic.Bool
	readError  error
	cancel     context.CancelFunc
	closeOnce  sync.Once
}

func newResponsesWebsocketConnection(connection *websocket.Conn) *responsesWebsocketConnection {
	operationContext, cancel := context.WithCancel(context.Background())
	socket := &responsesWebsocketConnection{connection: connection, opened: time.Now(), events: make(chan []byte, 1), done: make(chan struct{}), cancel: cancel}
	connection.SetReadLimit(maximumResponsesEventBytes)
	go socket.readEvents(operationContext)
	return socket
}

func (socket *responsesWebsocketConnection) readEvents(operationContext context.Context) {
	defer close(socket.done)
	defer close(socket.events)
	defer socket.closed.Store(true)
	for {
		messageType, data, operationError := socket.connection.Read(operationContext)
		if operationError != nil {
			socket.readError = operationError
			return
		}
		if messageType != websocket.MessageText {
			socket.readError = fmt.Errorf("unexpected binary Responses WebSocket event")
			return
		}
		var envelope struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(data, &envelope) == nil && envelope.Type != "error" && !strings.HasPrefix(envelope.Type, "response.") {
			// Rate-limit and telemetry events do not belong to model history and
			// must not block control-frame handling between model requests.
			continue
		}
		select {
		case socket.events <- data:
		case <-operationContext.Done():
			socket.readError = operationContext.Err()
			return
		}
	}
}

func (socket *responsesWebsocketConnection) nextEvent(operationContext context.Context) ([]byte, error) {
	select {
	case <-operationContext.Done():
		return nil, operationContext.Err()
	case data, available := <-socket.events:
		if available {
			return data, nil
		}
		if socket.readError != nil {
			return nil, socket.readError
		}
		return nil, io.ErrUnexpectedEOF
	}
}

func (socket *responsesWebsocketConnection) reusable() bool {
	if socket.closed.Load() || time.Since(socket.opened) >= responsesConnectionAgeLimit {
		return false
	}
	select {
	case <-socket.events:
		return false // Unconsumed response data cannot belong to a new request.
	default:
		return true
	}
}

func (socket *responsesWebsocketConnection) close() {
	socket.closeOnce.Do(func() {
		socket.cancel()
		// This is local transport cancellation, not an acknowledgement that the
		// provider stopped generation. Read/Write errors reach the active stream.
		socket.connection.CloseNow()
	})
	<-socket.done
}
