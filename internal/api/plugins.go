package api

import (
	"encoding/json"
	"net/http"

	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func (server *Server) pluginScope(responseWriter http.ResponseWriter, request *http.Request) (string, bool) {
	workspaceID := request.PathValue("id")
	if workspaceID != "" {
		if _, operationError := server.store.Instances().Get(request.Context(), workspaceID); respondToError(responseWriter, http.StatusNotFound, operationError) {
			return "", false
		}
	}
	return workspaceID, true
}

func (server *Server) listPlugins(responseWriter http.ResponseWriter, request *http.Request) {
	workspaceID, valid := server.pluginScope(responseWriter, request)
	if !valid {
		return
	}
	states := []harness.PluginState{}
	if server.plugins != nil {
		for _, plugin := range server.plugins.All() {
			state := harness.PluginState{Name: plugin.Name(), Version: plugin.Version(), WorkspaceID: workspaceID, Available: true, Enabled: true, RequestedEnabled: true}
			if configurable, supported := plugin.(harness.ConfigurablePlugin); supported {
				var operationError error
				state, operationError = configurable.ReadState(request.Context(), workspaceID)
				if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
					return
				}
			}
			states = append(states, state)
		}
	}
	writeJSON(responseWriter, http.StatusOK, states)
}

func (server *Server) resolvePlugin(responseWriter http.ResponseWriter, request *http.Request) (harness.Plugin, string, bool) {
	workspaceID, valid := server.pluginScope(responseWriter, request)
	if !valid {
		return nil, "", false
	}
	if server.plugins == nil {
		writeError(responseWriter, http.StatusNotFound, "plugin is not registered")
		return nil, "", false
	}
	plugin, found := server.plugins.Get(request.PathValue("plugin"))
	if !found {
		writeError(responseWriter, http.StatusNotFound, "plugin is not registered")
		return nil, "", false
	}
	return plugin, workspaceID, true
}

func (server *Server) pluginSettings(responseWriter http.ResponseWriter, request *http.Request) {
	plugin, workspaceID, valid := server.resolvePlugin(responseWriter, request)
	if !valid {
		return
	}
	configurable, supported := plugin.(harness.ConfigurablePlugin)
	if !supported {
		writeError(responseWriter, http.StatusBadRequest, "plugin has no runtime settings")
		return
	}
	state, operationError := configurable.ReadState(request.Context(), workspaceID)
	if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, state)
}

func (server *Server) updatePluginSettings(responseWriter http.ResponseWriter, request *http.Request) {
	plugin, workspaceID, valid := server.resolvePlugin(responseWriter, request)
	if !valid {
		return
	}
	configurable, supported := plugin.(harness.ConfigurablePlugin)
	if !supported {
		writeError(responseWriter, http.StatusBadRequest, "plugin has no runtime settings")
		return
	}
	var input json.RawMessage
	if respondToError(responseWriter, http.StatusBadRequest, readJSON(request, &input)) {
		return
	}
	state, operationError := configurable.UpdateConfiguration(request.Context(), workspaceID, input)
	if respondToError(responseWriter, http.StatusBadRequest, operationError) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, state)
}

func (server *Server) pluginStatistics(responseWriter http.ResponseWriter, request *http.Request) {
	plugin, workspaceID, valid := server.resolvePlugin(responseWriter, request)
	if !valid {
		return
	}
	observable, supported := plugin.(harness.ObservablePlugin)
	if !supported {
		writeError(responseWriter, http.StatusBadRequest, "plugin has no usage metrics")
		return
	}
	metrics, operationError := observable.Metrics(request.Context(), workspaceID)
	if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, metrics)
}

func (server *Server) workspaceAgentStatistics(responseWriter http.ResponseWriter, request *http.Request) {
	workspaceID, valid := server.pluginScope(responseWriter, request)
	if !valid {
		return
	}
	statistics, operationError := server.store.Usage().Agents(request.Context(), workspaceID)
	if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, statistics)
}
