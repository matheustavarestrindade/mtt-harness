package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

func (server *Server) sessionHistory(responseWriter http.ResponseWriter, request *http.Request) {
	query := store.HistoryQuery{}
	for key, target := range map[string]*int64{"before": &query.Before, "after": &query.After} {
		if value := request.URL.Query().Get(key); value != "" {
			parsed, operationError := strconv.ParseInt(value, 10, 64)
			if respondToError(responseWriter, http.StatusBadRequest, operationError) {
				return
			}
			*target = parsed
		}
	}
	if value := request.URL.Query().Get("limit"); value != "" {
		parsed, operationError := strconv.Atoi(value)
		if respondToError(responseWriter, http.StatusBadRequest, operationError) {
			return
		}
		query.Limit = parsed
	}
	if value := request.URL.Query().Get("revision"); value != "" {
		parsed, operationError := strconv.ParseUint(value, 10, 64)
		if respondToError(responseWriter, http.StatusBadRequest, operationError) {
			return
		}
		query.Revision = &parsed
	}
	query.MetadataOnly = request.URL.Query().Get("metadata") == "true"
	query, operationError := query.Validate()
	if respondToError(responseWriter, http.StatusBadRequest, operationError) {
		return
	}
	page, operationError := server.store.Sessions().History(request.Context(), atom.SessionID(request.PathValue("id")), query)
	status := http.StatusInternalServerError
	if errors.Is(operationError, store.ErrSessionNotFound) || errors.Is(operationError, store.ErrSessionDeleted) {
		status = http.StatusNotFound
	}
	if errors.Is(operationError, store.ErrHistoryChanged) {
		status = http.StatusConflict
	}
	if respondToError(responseWriter, status, operationError) {
		return
	}
	if !query.MetadataOnly {
		var identifiers []string
		for _, message := range page.Messages {
			identifiers = append(identifiers, message.ID)
		}
		for _, message := range page.Context {
			identifiers = append(identifiers, message.ID)
		}
		page.Processes, operationError = server.store.Processes().ForMessages(request.Context(), page.SessionID, identifiers)
		if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
			return
		}
	}
	writeJSON(responseWriter, http.StatusOK, page)
}
