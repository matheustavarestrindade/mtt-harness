package api

import (
	"net/http"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (server *Server) sessionProcesses(responseWriter http.ResponseWriter, request *http.Request) {
	records, operationError := server.store.Processes().List(request.Context(), atom.SessionID(request.PathValue("id")))
	if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
		return
	}
	if records == nil {
		records = []atom.ProcessRecord{}
	}
	writeJSON(responseWriter, http.StatusOK, records)
}
