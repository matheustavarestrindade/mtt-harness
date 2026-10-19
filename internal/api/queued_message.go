package api

import (
	"errors"
	"net/http"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

func (server *Server) queuedMessage(responseWriter http.ResponseWriter, request *http.Request) {
	sessionID := atom.SessionID(request.PathValue("id"))
	_, operationError := server.store.Sessions().Get(request.Context(), sessionID)
	if respondToError(responseWriter, http.StatusNotFound, operationError) {
		return
	}
	message, operationError := server.store.Queue().PendingMessage(request.Context(), sessionID, request.PathValue("message_id"))
	status := http.StatusInternalServerError
	if errors.Is(operationError, store.ErrQueuedMessageNotFound) {
		status = http.StatusNotFound
	}
	if respondToError(responseWriter, status, operationError) {
		return
	}
	responseWriter.Header().Set("Cache-Control", "no-store")
	writeJSON(responseWriter, http.StatusOK, message)
}
