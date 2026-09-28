package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
)

type instanceRequest struct {
	Workspace       string   `json:"workspace"`
	Models          []string `json:"models"`
	DefaultModel    string   `json:"default_model"`
	ProcessLimit    int      `json:"process_limit"`
	AgentDepthLimit int      `json:"agent_depth_limit"`
}

func (server *Server) startInstance(responseWriter http.ResponseWriter, request *http.Request) {
	var input instanceRequest
	if respondToError(responseWriter, http.StatusBadRequest, readJSON(request, &input)) {
		return
	}
	instanceSpec := atom.InstanceSpec{
		Workspace:       input.Workspace,
		Models:          input.Models,
		DefaultModel:    input.DefaultModel,
		ProcessLimit:    input.ProcessLimit,
		AgentDepthLimit: input.AgentDepthLimit,
		CreatedAt:       time.Now(),
	}
	instance, operationError := server.instances.Start(request.Context(), instanceSpec)
	if respondToError(responseWriter, http.StatusBadRequest, operationError) {
		return
	}
	if respondToError(responseWriter, http.StatusInternalServerError, server.store.Instances().Save(request.Context(), instance.Spec())) {
		return
	}
	writeJSON(responseWriter, http.StatusCreated, instance.Spec())
}

func (server *Server) listInstances(responseWriter http.ResponseWriter, request *http.Request) {
	instances, operationError := server.store.Instances().All(request.Context())
	if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, instances)
}

func (server *Server) getInstance(responseWriter http.ResponseWriter, request *http.Request) {
	instance, operationError := server.store.Instances().Get(request.Context(), request.PathValue("id"))
	if respondToError(responseWriter, http.StatusNotFound, operationError) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, instance)
}

func (server *Server) stopInstance(responseWriter http.ResponseWriter, request *http.Request) {
	identifier := request.PathValue("id")
	instance, operationError := server.store.Instances().Get(request.Context(), identifier)
	if respondToError(responseWriter, http.StatusNotFound, operationError) {
		return
	}
	instance.Stopped = true
	if respondToError(responseWriter, http.StatusInternalServerError, server.store.Instances().Save(request.Context(), instance)) {
		return
	}
	if server.instances.IsRunning(identifier) {
		if respondToError(responseWriter, http.StatusInternalServerError, server.instances.Stop(request.Context(), identifier)) {
			return
		}
	}
	var stopError error
	if server.queue != nil {
		stopError = server.queue.StopInstance(request.Context(), identifier)
	}
	if server.processes != nil {
		_, processError := server.processes.StopInstance(request.Context(), identifier)
		stopError = errors.Join(stopError, processError)
	}
	if respondToError(responseWriter, http.StatusInternalServerError, stopError) {
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]string{"status": "stopped"})
}

func (server *Server) resumeInstance(responseWriter http.ResponseWriter, request *http.Request) {
	instanceSpec, operationError := server.store.Instances().Get(request.Context(), request.PathValue("id"))
	if respondToError(responseWriter, http.StatusNotFound, operationError) {
		return
	}
	instanceSpec.Stopped = false
	instance, operationError := server.instances.Start(request.Context(), instanceSpec)
	if respondToError(responseWriter, http.StatusBadRequest, operationError) {
		return
	}
	if respondToError(responseWriter, http.StatusInternalServerError, server.store.Instances().Save(request.Context(), instance.Spec())) {
		return
	}
	if server.queue != nil {
		server.queue.ResumeInstance(instance.ID())
	}
	writeJSON(responseWriter, http.StatusOK, instance.Spec())
}

func (server *Server) instanceModels(responseWriter http.ResponseWriter, request *http.Request) {
	instance, found := server.instances.Get(request.PathValue("id"))
	if !found {
		writeError(responseWriter, http.StatusNotFound, "the instance is not in the manager")
		return
	}
	writeJSON(responseWriter, http.StatusOK, server.modelsOf(instance))
}

func (server *Server) modelsOf(instance *instances.Instance) []atom.ModelInfo {
	allowed := map[string]bool{}
	for _, identifier := range instance.Spec().Models {
		model, provider, operationError := server.gateway.Resolve(identifier)
		if operationError == nil {
			allowed[provider.Name()+"/"+model.ID] = true
		}
	}
	var result []atom.ModelInfo
	for _, provider := range server.gateway.Providers() {
		for _, model := range provider.Models() {
			model.ID = provider.Name() + "/" + model.ID
			if len(instance.Spec().Models) > 0 && !allowed[model.ID] {
				continue
			}
			result = append(result, model)
		}
	}
	return result
}

func (server *Server) listSessions(responseWriter http.ResponseWriter, request *http.Request) {
	instanceID := request.PathValue("id")
	if _, operationError := server.store.Instances().Get(request.Context(), instanceID); respondToError(responseWriter, http.StatusNotFound, operationError) {
		return
	}
	sessions, operationError := server.store.Sessions().List(request.Context(), instanceID)
	if respondToError(responseWriter, http.StatusInternalServerError, operationError) {
		return
	}
	if sessions == nil {
		sessions = []atom.Session{}
	}
	writeJSON(responseWriter, http.StatusOK, sessions)
}

func (server *Server) sessionsOf(operationContext context.Context, instance *instances.Instance) []atom.Session {
	identifiers, _ := instance.Sessions().Agents(operationContext, "")
	var sessions []atom.Session
	for _, sessionID := range identifiers {
		if session, found := instance.Sessions().Get(operationContext, sessionID); found && session.InstanceID == instance.ID() {
			sessions = append(sessions, session)
		}
	}
	return sessions
}
