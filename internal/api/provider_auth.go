package api

import (
	"net/http"
)

func (server *Server) supportsDeviceLogin(responseWriter http.ResponseWriter, request *http.Request) bool {
	responseWriter.Header().Set("Cache-Control", "no-store")
	if server.gateway == nil {
		writeError(responseWriter, http.StatusServiceUnavailable, "provider gateway is not available")
		return false
	}
	registeredProvider, found := server.gateway.Provider(request.PathValue("id"))
	if !found {
		writeError(responseWriter, http.StatusNotFound, "provider is not configured")
		return false
	}
	switch providerSpecification(registeredProvider).Authentication {
	case "chatgpt":
	default:
		writeError(responseWriter, http.StatusBadRequest, "provider does not use device authentication")
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
	login, operationError := server.providerAuth.StartDeviceLogin(request.Context(), request.PathValue("id"))
	if respondToError(responseWriter, http.StatusBadGateway, operationError) {
		return
	}
	writeJSON(responseWriter, http.StatusAccepted, login)
}

func (server *Server) providerLoginStatus(responseWriter http.ResponseWriter, request *http.Request) {
	if !server.supportsDeviceLogin(responseWriter, request) {
		return
	}
	login, operationError := server.providerAuth.DeviceLoginStatus(request.Context(), request.PathValue("id"), request.PathValue("login_id"))
	if respondToError(responseWriter, http.StatusNotFound, operationError) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, login)
}

func (server *Server) cancelProviderLogin(responseWriter http.ResponseWriter, request *http.Request) {
	if !server.supportsDeviceLogin(responseWriter, request) {
		return
	}
	if respondToError(responseWriter, http.StatusNotFound, server.providerAuth.CancelDeviceLogin(request.Context(), request.PathValue("id"), request.PathValue("login_id"))) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (server *Server) disconnectProvider(responseWriter http.ResponseWriter, request *http.Request) {
	if !server.supportsDeviceLogin(responseWriter, request) {
		return
	}
	if respondToError(responseWriter, http.StatusInternalServerError, server.providerAuth.DisconnectProvider(request.Context(), request.PathValue("id"))) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]string{"status": "disconnected"})
}
