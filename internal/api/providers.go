package api

import (
	"net/http"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func (server *Server) listProviders(responseWriter http.ResponseWriter, request *http.Request) {
	connections := []atom.ProviderConnection{}
	if server.gateway == nil {
		writeError(responseWriter, http.StatusServiceUnavailable, "provider gateway is not available")
		return
	}
	for _, registered := range server.gateway.Providers() {
		specification := providerSpecification(registered)
		connection := atom.ProviderConnection{ProviderSpec: specification, ModelCount: len(registered.Models()), Connected: true}
		if specification.Authentication == "chatgpt" {
			credential, operationError := server.store.Secrets().OAuthCredential(request.Context(), registered.Name())
			if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
				return
			}
			connection.Connected = credential.AccessToken != "" && credential.RefreshToken != ""
		}
		if specification.Authentication == "api_key" {
			key, operationError := server.store.Secrets().ProviderKey(request.Context(), registered.Name())
			if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
				return
			}
			connection.Connected = key != ""
		}
		connections = append(connections, connection)
	}
	responseWriter.Header().Set("Cache-Control", "no-store")
	writeJSON(responseWriter, http.StatusOK, connections)
}

func providerSpecification(registered harness.Provider) atom.ProviderSpec {
	if described, supported := registered.(interface{ Spec() atom.ProviderSpec }); supported {
		return described.Spec()
	}
	return atom.ProviderSpec{Name: registered.Name(), Authentication: "none"}
}

func (server *Server) providerModels(responseWriter http.ResponseWriter, request *http.Request) {
	if server.gateway == nil {
		writeError(responseWriter, http.StatusServiceUnavailable, "provider gateway is not available")
		return
	}
	registered, found := server.gateway.Provider(request.PathValue("id"))
	if !found {
		writeError(responseWriter, http.StatusNotFound, "provider is not registered")
		return
	}
	models := registered.Models()
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
