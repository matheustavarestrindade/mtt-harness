package api

import (
	"net/http"
	"strconv"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (server *Server) sessionProcesses(responseWriter http.ResponseWriter, request *http.Request) {
	var records []atom.ProcessRecord
	var operationError error
	if value := request.URL.Query().Get("limit"); value != "" {
		limit, parseError := strconv.Atoi(value)
		if respondToError(responseWriter, http.StatusBadRequest, parseError) {
			return
		}
		records, operationError = server.store.Processes().Recent(request.Context(), atom.SessionID(request.PathValue("id")), limit)
	} else {
		records, operationError = server.store.Processes().List(request.Context(), atom.SessionID(request.PathValue("id")))
	}
	if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
		return
	}
	if records == nil {
		records = []atom.ProcessRecord{}
	}
	writeJSON(responseWriter, http.StatusOK, records)
}
