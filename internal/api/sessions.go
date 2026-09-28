package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (server *Server) startSession(responseWriter http.ResponseWriter, request *http.Request) {
	instance, found := server.instances.Get(request.PathValue("id"))
	if !found {
		writeError(responseWriter, http.StatusNotFound, "the instance is not in the manager")
		return
	}
	var input struct {
		Model string `json:"model"`
	}
	if operationError := readJSON(request, &input); operationError != nil && !errors.Is(operationError, io.EOF) {
		writeError(responseWriter, http.StatusBadRequest, operationError.Error())
		return
	}
	model := input.Model
	if model == "" {
		model = instance.Spec().DefaultModel
	}
	modelInfo, modelProvider, operationError := server.gateway.ResolveAllowed(model, instance.Spec().Models)
	if respondToError(responseWriter, http.StatusBadRequest, operationError) {
		return
	}
	model = modelProvider.Name() + "/" + modelInfo.ID
	session := atom.Session{
		ID:         atom.SessionID(newID()),
		InstanceID: instance.ID(),
		Model:      model,
		CreatedAt:  time.Now(),
	}
	if respondToError(responseWriter, http.StatusInternalServerError, server.store.Sessions().Save(request.Context(), session)) {
		return
	}
	writeJSON(responseWriter, http.StatusCreated, session)
}

func (server *Server) getSession(responseWriter http.ResponseWriter, request *http.Request) {
	session, operationError := server.store.Sessions().Get(request.Context(), atom.SessionID(request.PathValue("id")))
	if respondToError(responseWriter, http.StatusNotFound, operationError) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, session)
}

func (server *Server) sessionAgents(responseWriter http.ResponseWriter, request *http.Request) {
	agents, operationError := server.store.Sessions().Agents(request.Context(), atom.SessionID(request.PathValue("id")))
	if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
		return
	}
	if agents == nil {
		agents = []atom.SessionID{}
	}
	writeJSON(responseWriter, http.StatusOK, agents)
}

func newID() string {
	var data [16]byte
	if _, operationError := rand.Read(data[:]); operationError != nil {
		return "unknown"
	}
	return hex.EncodeToString(data[:])
}
