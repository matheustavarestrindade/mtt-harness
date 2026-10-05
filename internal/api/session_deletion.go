package api

import (
	"errors"
	"net/http"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
)

func (server *Server) deleteSession(responseWriter http.ResponseWriter, request *http.Request) {
	if server.queue == nil {
		writeError(responseWriter, http.StatusServiceUnavailable, "session lifecycle control is unavailable")
		return
	}
	session, operationError := server.store.Sessions().Get(request.Context(), atom.SessionID(request.PathValue("id")))
	if operationError != nil {
		status := http.StatusInternalServerError
		if errors.Is(operationError, store.ErrSessionDeleted) || errors.Is(operationError, store.ErrSessionNotFound) {
			status = http.StatusNotFound
		}
		writeError(responseWriter, status, operationError.Error())
		return
	}
	deleted, operationError := server.queue.DeleteConversation(request.Context(), session)
	if operationError != nil {
		status := http.StatusInternalServerError
		if errors.Is(operationError, store.ErrSessionDeleted) || errors.Is(operationError, store.ErrSessionNotFound) {
			status = http.StatusNotFound
		}
		if errors.Is(operationError, store.ErrConversationBusy) || errors.Is(operationError, loop.ErrSessionBusy) || errors.Is(operationError, loop.ErrQueueClosed) {
			status = http.StatusConflict
		}
		writeError(responseWriter, status, operationError.Error())
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]any{"status": "deleted", "session_ids": deleted})
}
