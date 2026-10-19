package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func (server *Server) sessionPluginActivity(responseWriter http.ResponseWriter, request *http.Request) {
	session, operationError := server.store.Sessions().Get(request.Context(), atom.SessionID(request.PathValue("id")))
	if respondToError(responseWriter, http.StatusNotFound, operationError) {
		return
	}
	if server.plugins == nil {
		writeError(responseWriter, http.StatusNotFound, "plugin is not registered")
		return
	}
	plugin, found := server.plugins.Get(request.PathValue("plugin"))
	if !found {
		writeError(responseWriter, http.StatusNotFound, "plugin is not registered")
		return
	}
	activity, available := plugin.(harness.SessionActivityPlugin)
	if !available {
		writeError(responseWriter, http.StatusNotFound, "plugin has no session activity")
		return
	}
	query := harness.PluginActivityQuery{Cursor: request.URL.Query().Get("cursor"), Limit: 50}
	if value := request.URL.Query().Get("limit"); value != "" {
		query.Limit, operationError = strconv.Atoi(value)
		if operationError != nil || query.Limit < 1 || query.Limit > 100 {
			writeError(responseWriter, http.StatusBadRequest, "activity limit must be 1–100")
			return
		}
	}
	if len(query.Cursor) > 2048 {
		writeError(responseWriter, http.StatusBadRequest, "activity cursor exceeds 2048 bytes")
		return
	}
	page, operationError := activity.SessionActivity(request.Context(), session, query)
	if errors.Is(operationError, harness.ErrPluginActivityCursor) {
		writeError(responseWriter, http.StatusBadRequest, operationError.Error())
		return
	}
	if errors.Is(operationError, harness.ErrPluginActivityChanged) {
		writeError(responseWriter, http.StatusConflict, operationError.Error())
		return
	}
	if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
		return
	}
	if page.Entries == nil {
		page.Entries = []harness.PluginActivityEntry{}
	}
	writeJSON(responseWriter, http.StatusOK, page)
}
