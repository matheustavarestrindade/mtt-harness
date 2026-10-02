package api

import (
	"errors"
	"net/http"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

func (server *Server) setSessionModel(responseWriter http.ResponseWriter, request *http.Request) {
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
		Model           string `json:"model"`
		AllowCompaction bool   `json:"allow_compaction"`
	}
	if operationError := readJSON(request, &input); operationError != nil {
		writeError(responseWriter, http.StatusBadRequest, operationError.Error())
		return
	}
	if input.Model == "" {
		writeError(responseWriter, http.StatusBadRequest, "model is required")
		return
	}
	target, modelProvider, operationError := server.gateway.ResolveAllowed(input.Model, instance.Spec().Models)
	if respondToError(responseWriter, http.StatusBadRequest, operationError) {
		return
	}
	currentID := session.Model
	if currentID == "" {
		currentID = instance.Spec().DefaultModel
	}
	current, currentProvider, currentError := server.gateway.Resolve(currentID)
	targetID := modelProvider.Name() + "/" + target.ID
	sameModel := currentError == nil && currentProvider.Name()+"/"+current.ID == targetID
	if !sameModel && target.ContextMax <= 0 {
		writeError(responseWriter, http.StatusBadRequest, "the selected model has no reported context size; refresh model data or configure context_max before switching")
		return
	}
	needsCompaction := !sameModel && (currentError != nil || current.ContextMax <= 0 || target.ContextMax < current.ContextMax)
	if needsCompaction && !input.AllowCompaction {
		writeJSON(responseWriter, http.StatusConflict, map[string]any{
			"code":                "context_compaction_required",
			"error":               "Switching to this model requires confirmation: the conversation context will be compacted to fit by omitting older complete turns as needed. Saved history remains available.",
			"current_context_max": current.ContextMax,
			"target_context_max":  target.ContextMax,
			"model":               targetID,
		})
		return
	}
	selection := atom.SessionModelSelection{Model: targetID, ReasoningEffort: session.ReasoningEffort}
	if target.ValidateReasoningEffort(selection.ReasoningEffort) != nil {
		selection.ReasoningEffort = ""
	}
	if respondToSessionSelectionError(responseWriter, server.store.Sessions().SetModelSelection(request.Context(), session.ID, session.ModelSelection(), selection)) {
		return
	}
	session.Model, session.ReasoningEffort = selection.Model, selection.ReasoningEffort
	writeJSON(responseWriter, http.StatusOK, session)
}

func respondToSessionSelectionError(responseWriter http.ResponseWriter, operationError error) bool {
	if errors.Is(operationError, store.ErrSessionSelectionChanged) {
		writeJSON(responseWriter, http.StatusConflict, map[string]string{"code": "session_selection_changed", "error": operationError.Error()})
		return true
	}
	return respondToError(responseWriter, http.StatusInternalServerError, operationError)
}
