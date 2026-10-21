package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func (server *Server) pluginAction(responseWriter http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("id")
	if _, operationError := server.store.Instances().Get(request.Context(), workspaceID); respondToError(responseWriter, http.StatusNotFound, operationError) {
		return
	}
	if server.plugins == nil {
		writeError(responseWriter, http.StatusNotFound, "plugin is not available")
		return
	}
	registered, found := server.plugins.Get(request.PathValue("plugin"))
	if !found {
		writeError(responseWriter, http.StatusNotFound, "plugin is not available")
		return
	}
	actions, found := registered.(harness.PluginActions)
	if !found {
		writeError(responseWriter, http.StatusNotFound, "plugin actions are not available")
		return
	}
	var input json.RawMessage
	operationError := readJSON(request, &input)
	if operationError != nil && !errors.Is(operationError, io.EOF) {
		writeError(responseWriter, http.StatusBadRequest, operationError.Error())
		return
	}
	result, operationError := actions.Action(request.Context(), workspaceID, request.PathValue("action"), input)
	status := http.StatusBadRequest
	if errors.Is(operationError, harness.ErrPluginActionNotFound) {
		status = http.StatusNotFound
	}
	if respondToError(responseWriter, status, operationError) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, result)
}
