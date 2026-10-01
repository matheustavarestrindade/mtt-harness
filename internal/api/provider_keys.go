package api

import (
	"net/http"
	"strings"
)

func (server *Server) saveProviderKey(responseWriter http.ResponseWriter, request *http.Request) {
	if server.gateway != nil {
		if registeredProvider, found := server.gateway.Provider(request.PathValue("id")); found {
			switch providerSpecification(registeredProvider).Authentication {
			case "chatgpt":
				writeError(responseWriter, http.StatusBadRequest, "use ChatGPT device sign-in for this provider")
				return
			}
		}
	}
	var input struct {
		Key string `json:"key"`
	}
	if respondToError(responseWriter, http.StatusBadRequest, readJSON(request, &input)) {
		return
	}
	input.Key = strings.TrimSpace(input.Key)
	if input.Key == "" {
		writeError(responseWriter, http.StatusBadRequest, "the API key must not be empty; use DELETE to disconnect")
		return
	}
	if respondToError(responseWriter, http.StatusInternalServerError, server.store.Secrets().SaveProviderKey(request.Context(), request.PathValue("id"), input.Key)) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]string{"status": "saved"})
}

func (server *Server) deleteProviderKey(responseWriter http.ResponseWriter, request *http.Request) {
	if respondToError(responseWriter, http.StatusInternalServerError, server.store.Secrets().DeleteProviderKey(request.Context(), request.PathValue("id"))) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]string{"status": "deleted"})
}

func (server *Server) saveInstanceKey(responseWriter http.ResponseWriter, request *http.Request) {
	var input struct {
		Key string `json:"key"`
	}
	if respondToError(responseWriter, http.StatusBadRequest, readJSON(request, &input)) {
		return
	}
	if respondToError(responseWriter, http.StatusInternalServerError, server.store.Secrets().SaveInstanceKey(request.Context(), request.PathValue("id"), request.PathValue("provider"), input.Key)) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]string{"status": "saved"})
}

func (server *Server) deleteInstanceKey(responseWriter http.ResponseWriter, request *http.Request) {
	if respondToError(responseWriter, http.StatusInternalServerError, server.store.Secrets().DeleteInstanceKey(request.Context(), request.PathValue("id"), request.PathValue("provider"))) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]string{"status": "deleted"})
}
