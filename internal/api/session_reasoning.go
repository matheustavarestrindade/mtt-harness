package api

import (
	"net/http"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (server *Server) setSessionReasoningEffort(responseWriter http.ResponseWriter, request *http.Request) {
	session, operationError := server.store.Sessions().Get(request.Context(), atom.SessionID(request.PathValue("id")))
	if respondToError(responseWriter, http.StatusNotFound, operationError) {
		return
	}
	if session.Completed {
		writeError(responseWriter, http.StatusConflict, "the child session has completed")
		return
	}
	instance, found := server.instances.Get(session.InstanceID)
	if !found {
		writeError(responseWriter, http.StatusConflict, "the instance is stopped")
		return
	}
	var input struct {
		Effort *string `json:"effort"`
	}
	if operationError := readJSON(request, &input); operationError != nil {
		writeError(responseWriter, http.StatusBadRequest, operationError.Error())
		return
	}
	if input.Effort == nil {
		writeError(responseWriter, http.StatusBadRequest, "effort is required; use an empty string for the model default")
		return
	}
	modelID := session.Model
	if modelID == "" {
		modelID = instance.Spec().DefaultModel
	}
	model, _, operationError := server.gateway.ResolveAllowed(modelID, instance.Spec().Models)
	if respondToError(responseWriter, http.StatusBadRequest, operationError) {
		return
	}
	if respondToError(responseWriter, http.StatusBadRequest, model.ValidateReasoningEffort(*input.Effort)) {
		return
	}
	if respondToError(responseWriter, http.StatusInternalServerError, server.store.Sessions().SetReasoningEffort(request.Context(), session.ID, *input.Effort)) {
		return
	}
	session.ReasoningEffort = *input.Effort
	writeJSON(responseWriter, http.StatusOK, session)
}
