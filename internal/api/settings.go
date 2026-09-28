package api

import (
	"net/http"
	"strconv"
)

func (server *Server) globalSettings(responseWriter http.ResponseWriter, request *http.Request) {
	values, operationError := server.store.Settings().All(request.Context(), "")
	if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, values)
}

func (server *Server) saveGlobalSetting(responseWriter http.ResponseWriter, request *http.Request) {
	server.saveSetting(responseWriter, request, "")
}

func (server *Server) deleteGlobalSetting(responseWriter http.ResponseWriter, request *http.Request) {
	server.deleteSetting(responseWriter, request, "")
}

func (server *Server) instanceSettings(responseWriter http.ResponseWriter, request *http.Request) {
	values, operationError := server.store.Settings().All(request.Context(), request.PathValue("id"))
	if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, values)
}

func (server *Server) saveInstanceSetting(responseWriter http.ResponseWriter, request *http.Request) {
	server.saveSetting(responseWriter, request, request.PathValue("id"))
}

func (server *Server) deleteInstanceSetting(responseWriter http.ResponseWriter, request *http.Request) {
	server.deleteSetting(responseWriter, request, request.PathValue("id"))
}

func (server *Server) saveSetting(responseWriter http.ResponseWriter, request *http.Request, scope string) {
	var input struct {
		Value string `json:"value"`
	}
	if respondToError(responseWriter, http.StatusBadRequest, readJSON(request, &input)) {
		return
	}
	switch request.PathValue("key") {
	case "agent_depth_limit", "process_limit":
		value, operationError := strconv.Atoi(input.Value)
		if operationError != nil || value < 0 {
			writeError(responseWriter, http.StatusBadRequest, "limit must be a non-negative integer")
			return
		}
	case "api_token":
		if scope != "" || input.Value == "" {
			writeError(responseWriter, http.StatusBadRequest, "api_token is a non-empty harness setting")
			return
		}
	default:
		writeError(responseWriter, http.StatusBadRequest, "unknown setting")
		return
	}
	if respondToError(responseWriter, http.StatusInternalServerError, server.store.Settings().Save(request.Context(), scope, request.PathValue("key"), input.Value)) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]string{"status": "saved"})
}

func (server *Server) deleteSetting(responseWriter http.ResponseWriter, request *http.Request, scope string) {
	if request.PathValue("key") == "api_token" {
		writeError(responseWriter, http.StatusBadRequest, "replace the API token with PUT")
		return
	}
	if respondToError(responseWriter, http.StatusInternalServerError, server.store.Settings().Delete(request.Context(), scope, request.PathValue("key"))) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]string{"status": "deleted"})
}
