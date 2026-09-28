package api

import (
	"net/http"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (server *Server) listProviders(responseWriter http.ResponseWriter, request *http.Request) {
	specifications, operationError := server.store.Providers().All(request.Context())
	if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
		return
	}
	if specifications == nil {
		specifications = []atom.ProviderSpec{}
	}
	writeJSON(responseWriter, http.StatusOK, specifications)
}

func (server *Server) providerModels(responseWriter http.ResponseWriter, request *http.Request) {
	models, operationError := server.store.Providers().Models(request.Context(), request.PathValue("id"))
	if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
		return
	}
	if models == nil {
		models = []atom.ModelInfo{}
	}
	writeJSON(responseWriter, http.StatusOK, models)
}

func (server *Server) refreshProvider(responseWriter http.ResponseWriter, request *http.Request) {
	name := request.PathValue("id")
	models, operationError := server.gateway.Refresh(request.Context(), name)
	if respondToError(responseWriter, http.StatusBadGateway, operationError) {
		return
	}
	if respondToError(responseWriter, http.StatusInternalServerError, server.store.Providers().SaveModels(request.Context(), name, models)) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, models)
}
