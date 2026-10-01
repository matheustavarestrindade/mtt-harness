package api

import (
	"net/http"

	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
)

func (server *Server) supportsDeviceLogin(responseWriter http.ResponseWriter, request *http.Request) bool {
	responseWriter.Header().Set("Cache-Control", "no-store")
	if request.PathValue("id") != provider.CodexProvider {
		writeError(responseWriter, http.StatusBadRequest, "device sign-in is available for openai-codex only")
		return false
	}
	if server.providerAuth == nil {
		writeError(responseWriter, http.StatusServiceUnavailable, "provider authentication is not available")
		return false
	}
	return true
}

func (server *Server) startProviderLogin(responseWriter http.ResponseWriter, request *http.Request) {
	if !server.supportsDeviceLogin(responseWriter, request) {
		return
	}
	login, operationError := server.providerAuth.Start(request.Context())
	if respondToError(responseWriter, http.StatusBadGateway, operationError) {
		return
	}
	writeJSON(responseWriter, http.StatusAccepted, login)
}

func (server *Server) providerLoginStatus(responseWriter http.ResponseWriter, request *http.Request) {
	if !server.supportsDeviceLogin(responseWriter, request) {
		return
	}
	login, operationError := server.providerAuth.Status(request.Context(), request.PathValue("login_id"))
	if respondToError(responseWriter, http.StatusNotFound, operationError) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, login)
}

func (server *Server) cancelProviderLogin(responseWriter http.ResponseWriter, request *http.Request) {
	if !server.supportsDeviceLogin(responseWriter, request) {
		return
	}
	if respondToError(responseWriter, http.StatusNotFound, server.providerAuth.Cancel(request.Context(), request.PathValue("login_id"))) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (server *Server) disconnectProvider(responseWriter http.ResponseWriter, request *http.Request) {
	if !server.supportsDeviceLogin(responseWriter, request) {
		return
	}
	if respondToError(responseWriter, http.StatusInternalServerError, server.providerAuth.Disconnect(request.Context())) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]string{"status": "disconnected"})
}
