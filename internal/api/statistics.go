package api

import (
	"net/http"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (server *Server) sessionStatistics(responseWriter http.ResponseWriter, request *http.Request) {
	statistics, operationError := server.store.Usage().Session(request.Context(), atom.SessionID(request.PathValue("id")))
	if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, statistics)
}

func (server *Server) instanceStatistics(responseWriter http.ResponseWriter, request *http.Request) {
	statistics, operationError := server.store.Usage().Instance(request.Context(), request.PathValue("id"))
	if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, statistics)
}

func (server *Server) statistics(responseWriter http.ResponseWriter, request *http.Request) {
	statistics, operationError := server.store.Usage().All(request.Context())
	if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, statistics)
}
