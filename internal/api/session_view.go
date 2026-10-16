package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
)

type sessionViewState struct {
	Session atom.Session
	Status  struct {
		Running  bool     `json:"running"`
		Queued   int      `json:"queued"`
		Messages []string `json:"messages"`
		Error    string   `json:"error"`
	}
	Statistics       atom.Statistics
	TaskState        *atom.TaskState
	TaskStateError   string
	Outcome          string
	Permissions      []atom.PermissionRequest
	History          atom.HistoryPage
	Live             *atom.LiveResponse
	Processes        []atom.ProcessRecord
	WorkspaceLinked  bool
	WorkspaceStopped bool
	ActiveSessions   []atom.SessionID
	QueuedMessages   []atom.QueuedMessagePreview `json:",omitempty"`
}

func (server *Server) readSessionView(operationContext context.Context, sessionID atom.SessionID, details bool) (sessionViewState, error) {
	state := sessionViewState{Permissions: []atom.PermissionRequest{}, Processes: []atom.ProcessRecord{}}
	session, operationError := server.store.Sessions().Get(operationContext, sessionID)
	if operationError != nil {
		return state, operationError
	}
	state.Session = session
	instance, instanceError := server.store.Instances().Get(operationContext, session.InstanceID)
	if instanceError == nil {
		state.WorkspaceLinked = true
		state.WorkspaceStopped = instance.Stopped
	} else if !errors.Is(instanceError, store.ErrInstanceNotFound) {
		return state, instanceError
	}
	if server.queue != nil {
		state.ActiveSessions = server.queue.ActiveSessions(session.InstanceID)
		var status loop.QueueStatus
		if details {
			status, operationError = server.queue.StatusWithMessages(operationContext, sessionID)
		} else {
			status, operationError = server.queue.Status(operationContext, sessionID)
		}
		if operationError != nil && !errors.Is(operationError, loop.ErrQueueClosed) {
			return state, operationError
		}
		running, live := server.queue.LiveState(sessionID)
		state.Status.Running = status.Running || running
		state.Status.Messages = status.Messages
		state.Status.Queued = len(status.Messages)
		state.Status.Error = status.Error
		state.QueuedMessages = status.Previews
		if details {
			state.Live = live
		}
	}
	if state.Status.Messages == nil {
		state.Status.Messages = []string{}
	}
	state.History, operationError = server.store.Sessions().History(operationContext, sessionID, store.HistoryQuery{MetadataOnly: true})
	if operationError != nil {
		return state, operationError
	}
	if state.Live != nil && state.History.LatestResponse != nil && state.Live.Message.ID == state.History.LatestResponse.MessageID {
		state.Live = nil
	}
	state.Statistics, operationError = server.store.Usage().Session(operationContext, sessionID)
	if operationError != nil {
		return state, operationError
	}
	taskState, taskError := server.store.TaskStates().Get(operationContext, sessionID)
	if taskError == nil {
		state.TaskState = &taskState
	} else {
		state.TaskStateError = taskError.Error()
	}
	lastTurn, operationError := server.store.Events().LatestSessionEvent(operationContext, sessionID, atom.EventTurnEnd)
	if operationError != nil {
		return state, operationError
	}
	if lastTurn != nil && len(lastTurn.Payload) > 0 {
		var outcome struct {
			Status string `json:"status"`
		}
		if operationError = json.Unmarshal(lastTurn.Payload, &outcome); operationError != nil {
			return state, operationError
		}
		state.Outcome = outcome.Status
	}
	if server.broker != nil {
		state.Permissions = server.broker.SessionRequests(sessionID)
	}
	if details {
		state.Processes, operationError = server.store.Processes().Recent(operationContext, sessionID, 128)
	}
	return state, operationError
}

func (server *Server) sessionView(responseWriter http.ResponseWriter, request *http.Request) {
	state, operationError := server.readSessionView(request.Context(), atom.SessionID(request.PathValue("id")), request.URL.Query().Get("details") != "false")
	status := http.StatusInternalServerError
	if errors.Is(operationError, store.ErrSessionNotFound) || errors.Is(operationError, store.ErrSessionDeleted) {
		status = http.StatusNotFound
	}
	if respondToError(responseWriter, status, operationError) {
		return
	}
	responseWriter.Header().Set("Cache-Control", "no-store")
	writeJSON(responseWriter, http.StatusOK, state)
}

// sessionViewEvents subscribes before reading its snapshot. Live chunk revisions
// make the snapshot/event overlap idempotent. No old conversation is replayed:
// a small recent activity tail is display metadata, and history has its own pages.
func (server *Server) sessionViewEvents(responseWriter http.ResponseWriter, request *http.Request) {
	sessionID := atom.SessionID(request.PathValue("id"))
	session, operationError := server.store.Sessions().Get(request.Context(), sessionID)
	if respondToError(responseWriter, http.StatusNotFound, operationError) {
		return
	}
	if server.queue == nil || server.bus == nil {
		writeError(responseWriter, http.StatusServiceUnavailable, "session stream is unavailable")
		return
	}
	connection, operationError := websocket.Accept(responseWriter, request, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
	if operationError != nil {
		return
	}
	defer connection.Close(websocket.StatusNormalClosure, "")
	operationContext := connection.CloseRead(request.Context())
	changed := make(chan struct{}, 1)
	unsubscribeEvents := server.bus.On("*", func(_ context.Context, event atom.Event) {
		if event.SessionID != sessionID {
			return
		}
		select {
		case changed <- struct{}{}:
		default:
		}
	})
	defer unsubscribeEvents()
	stateChanged, unsubscribeState := server.queue.SubscribeSessionState(session)
	defer unsubscribeState()
	recent, operationError := server.store.Events().RecentSession(operationContext, sessionID, 80)
	if operationError != nil {
		return
	}
	var cursor uint64
	if len(recent) > 0 {
		cursor = recent[len(recent)-1].Seq
	}
	details := request.URL.Query().Get("details") != "false"
	var activity []atom.Event
	bytes := 0
	for index := len(recent) - 1; index >= 0; index-- {
		event := recent[index]
		if event.Name == atom.EventModelChunk || event.Name == atom.EventProcessOutput {
			continue
		}
		if bytes+len(event.Payload) > 262144 {
			break
		}
		bytes += len(event.Payload)
		activity = append([]atom.Event{event}, activity...)
	}
	sendState := func(initial bool) bool {
		latestEvents, latestError := server.store.Events().RecentSession(operationContext, sessionID, 1)
		if latestError != nil {
			return false
		}
		var covered uint64
		if len(latestEvents) > 0 {
			covered = latestEvents[0].Seq
		}
		state, operationError := server.readSessionView(operationContext, sessionID, details)
		if errors.Is(operationError, store.ErrSessionNotFound) || errors.Is(operationError, store.ErrSessionDeleted) {
			_ = sendJSON(operationContext, connection, map[string]any{"type": "deleted", "session_id": sessionID})
			return false
		}
		if operationError != nil {
			return false
		}
		frame := map[string]any{"type": "state", "session_id": sessionID, "cursor": cursor, "covered_cursor": covered, "state": state, "initial": initial}
		if initial {
			frame["recent"] = activity
		}
		return sendJSON(operationContext, connection, frame) == nil
	}
	if !sendState(true) {
		return
	}
	var stateTimer *time.Timer
	var stateReady <-chan time.Time
	defer func() {
		if stateTimer != nil {
			stateTimer.Stop()
		}
	}()
	requestState := func() {
		if stateReady == nil {
			stateTimer = time.NewTimer(75 * time.Millisecond)
			stateReady = stateTimer.C
		}
	}
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-operationContext.Done():
			return
		case <-stateChanged:
			requestState()
		case <-stateReady:
			stateReady = nil
			if !sendState(false) {
				return
			}
		case <-heartbeat.C:
			pingContext, cancel := context.WithTimeout(operationContext, 5*time.Second)
			operationError = connection.Ping(pingContext)
			cancel()
			if operationError != nil {
				return
			}
		case <-changed:
			for {
				events, operationError := server.store.Events().SinceSession(operationContext, sessionID, cursor)
				if operationError != nil {
					return
				}
				for _, event := range events {
					cursor = event.Seq
					if event.Name == atom.EventProcessOutput {
						continue
					}
					if event.Name != atom.EventModelChunk && event.Name != atom.EventActionReceived && event.Name != atom.EventToolStart && event.Name != atom.EventToolEnd {
						requestState()
					}
					if !details && (event.Name == atom.EventModelChunk || event.Name == atom.EventActionReceived || event.Name == atom.EventToolStart || event.Name == atom.EventToolEnd) {
						continue
					}
					if sendJSON(operationContext, connection, map[string]any{"type": "event", "event": event}) != nil {
						return
					}
				}
				if len(events) < 1000 {
					break
				}
			}
		}
	}
}
