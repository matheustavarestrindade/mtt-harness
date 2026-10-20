package api

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

func (server *Server) usageReport(responseWriter http.ResponseWriter, request *http.Request) {
	query, operationError := parseUsageQuery(request, time.Now().UTC())
	if respondToError(responseWriter, http.StatusBadRequest, operationError) {
		return
	}
	report, operationError := server.store.Usage().Report(request.Context(), query)
	if errors.Is(operationError, store.ErrUsageReportLimit) {
		writeError(responseWriter, http.StatusBadRequest, operationError.Error())
		return
	}
	if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
		return
	}
	responseWriter.Header().Set("Cache-Control", "no-store")
	writeJSON(responseWriter, http.StatusOK, report)
}

func parseUsageQuery(request *http.Request, now time.Time) (atom.UsageQuery, error) {
	values := request.URL.Query()
	for name, entries := range values {
		if name != "from" && name != "to" && name != "instance_id" && name != "session_id" && name != "agent_prefix" {
			return atom.UsageQuery{}, fmt.Errorf("unknown usage parameter %q", name)
		}
		if len(entries) != 1 {
			return atom.UsageQuery{}, fmt.Errorf("usage parameter %q has multiple values", name)
		}
	}
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	var operationError error
	if value := values.Get("to"); value != "" {
		end, operationError = time.Parse(time.DateOnly, value)
		if operationError != nil {
			return atom.UsageQuery{}, fmt.Errorf("invalid usage to date: %w", operationError)
		}
	}
	start := end.AddDate(0, 0, -29)
	if value := values.Get("from"); value != "" {
		start, operationError = time.Parse(time.DateOnly, value)
		if operationError != nil {
			return atom.UsageQuery{}, fmt.Errorf("invalid usage from date: %w", operationError)
		}
	}
	query := atom.UsageQuery{From: start, To: end, InstanceID: values.Get("instance_id"), SessionID: atom.SessionID(values.Get("session_id")), AgentPrefix: values.Get("agent_prefix")}
	return query, store.ValidateUsageQuery(query)
}
