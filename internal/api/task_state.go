package api

import (
	"errors"
	"net/http"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

func (server *Server) sessionTaskState(responseWriter http.ResponseWriter, request *http.Request) {
	sessionID := atom.SessionID(request.PathValue("id"))
	if _, operationError := server.store.Sessions().Get(request.Context(), sessionID); operationError != nil {
		status := http.StatusInternalServerError
		if errors.Is(operationError, store.ErrSessionDeleted) || errors.Is(operationError, store.ErrSessionNotFound) {
			status = http.StatusNotFound
		}
		respondToError(responseWriter, status, operationError)
		return
	}
	state, operationError := server.store.TaskStates().Get(request.Context(), sessionID)
	if errors.Is(operationError, store.ErrSessionDeleted) {
		respondToError(responseWriter, http.StatusNotFound, operationError)
		return
	}
	if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, state)
}
