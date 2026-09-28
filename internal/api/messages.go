package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
)

func (server *Server) sessionMessages(responseWriter http.ResponseWriter, request *http.Request) {
	messages, operationError := server.store.Sessions().Messages(request.Context(), atom.SessionID(request.PathValue("id")))
	if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
		return
	}
	if messages == nil {
		messages = []atom.Message{}
	}
	writeJSON(responseWriter, http.StatusOK, messages)
}

func (server *Server) sendMessage(responseWriter http.ResponseWriter, request *http.Request) {
	session, operationError := server.store.Sessions().Get(request.Context(), atom.SessionID(request.PathValue("id")))
	if respondToError(responseWriter, http.StatusNotFound, operationError) {
		return
	}
	if session.Completed {
		writeError(responseWriter, http.StatusConflict, "child agent is complete")
		return
	}
	var input struct {
		Content json.RawMessage `json:"content"`
	}
	if respondToError(responseWriter, http.StatusBadRequest, readJSON(request, &input)) {
		return
	}
	var text string
	var content []atom.Content
	if string(input.Content) == "null" {
		writeError(responseWriter, http.StatusBadRequest, "content is required")
		return
	}
	if json.Unmarshal(input.Content, &text) == nil {
		content = []atom.Content{{Type: atom.Text, Text: text}}
	}
	if content == nil && respondToError(responseWriter, http.StatusBadRequest, json.Unmarshal(input.Content, &content)) {
		return
	}
	if len(content) == 0 {
		writeError(responseWriter, http.StatusBadRequest, "content is required")
		return
	}
	message, position, operationError := server.queue.SubmitContent(request.Context(), session, content)
	status := http.StatusInternalServerError
	if errors.Is(operationError, loop.ErrInvalidContent) {
		status = http.StatusBadRequest
	}
	if errors.Is(operationError, store.ErrQueueFull) {
		status = http.StatusTooManyRequests
	}
	if errors.Is(operationError, loop.ErrSessionBusy) || errors.Is(operationError, loop.ErrInstanceStopped) || errors.Is(operationError, loop.ErrQueueClosed) {
		status = http.StatusConflict
	}
	if respondToError(responseWriter, status, operationError) {
		return
	}
	writeJSON(responseWriter, http.StatusAccepted, map[string]any{
		"status":   "queued",
		"position": position,
		"message":  message,
	})
}

func (server *Server) cancelMessage(responseWriter http.ResponseWriter, request *http.Request) {
	cancelled, operationError := server.queue.Cancel(request.Context(), atom.SessionID(request.PathValue("id")))
	if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
		return
	}
	if !cancelled {
		writeError(responseWriter, http.StatusConflict, "the session does not run")
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (server *Server) sessionStatus(responseWriter http.ResponseWriter, request *http.Request) {
	status, operationError := server.queue.Status(request.Context(), atom.SessionID(request.PathValue("id")))
	if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]any{
		"running":  status.Running,
		"queued":   len(status.Messages),
		"messages": status.Messages,
		"error":    status.Error,
	})
}

func (server *Server) cancelQueuedMessage(responseWriter http.ResponseWriter, request *http.Request) {
	removed, operationError := server.queue.CancelMessage(request.Context(), atom.SessionID(request.PathValue("id")), request.PathValue("message_id"))
	if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
		return
	}
	if !removed {
		writeError(responseWriter, http.StatusNotFound, "the message is not in the queue")
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]string{"status": "removed"})
}

func (server *Server) revertSession(responseWriter http.ResponseWriter, request *http.Request) {
	sessionID := atom.SessionID(request.PathValue("id"))
	session, operationError := server.store.Sessions().Get(request.Context(), sessionID)
	if respondToError(responseWriter, http.StatusNotFound, operationError) {
		return
	}
	var input struct {
		MessageID string `json:"message_id"`
	}
	if respondToError(responseWriter, http.StatusBadRequest, readJSON(request, &input)) {
		return
	}
	messages, operationError := server.store.Sessions().Messages(request.Context(), sessionID)
	if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
		return
	}
	found := false
	for _, message := range messages {
		if message.ID == input.MessageID {
			found = true
			break
		}
	}
	if !found {
		writeError(responseWriter, http.StatusNotFound, "the message is not in the session")
		return
	}
	removed, operationError := server.queue.Revert(request.Context(), session, input.MessageID)
	if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]any{"status": "reverted", "removed": removed})
}
