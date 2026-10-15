package api

import (
	"errors"
	"net/http"

	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
)

func (server *Server) unlinkInstance(responseWriter http.ResponseWriter, request *http.Request) {
	if server.queue == nil {
		writeError(responseWriter, http.StatusServiceUnavailable, "workspace lifecycle control is unavailable")
		return
	}
	identifier := request.PathValue("id")
	if respondToInstanceLifecycleError(responseWriter, server.queue.UnlinkInstance(request.Context(), identifier)) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]string{"id": identifier, "status": "unlinked"})
}

func respondToInstanceLifecycleError(responseWriter http.ResponseWriter, operationError error) bool {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(operationError, store.ErrInstanceNotFound), errors.Is(operationError, store.ErrInstanceUnlinked):
		status = http.StatusNotFound
	case errors.Is(operationError, store.ErrInstanceBusy), errors.Is(operationError, loop.ErrSessionBusy):
		status = http.StatusConflict
	case errors.Is(operationError, loop.ErrQueueClosed), errors.Is(operationError, loop.ErrInstanceLifecycleUnavailable):
		status = http.StatusServiceUnavailable
	case errors.Is(operationError, loop.ErrInstanceResume):
		status = http.StatusBadRequest
	}
	return respondToError(responseWriter, status, operationError)
}
