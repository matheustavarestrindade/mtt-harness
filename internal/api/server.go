package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/eventbus"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/permission"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/processes"
)

type Server struct {
	token     string
	store     store.Store
	instances *instances.Manager
	bus       *eventbus.Bus
	broker    *permission.Broker
	loop      *loop.Loop
	processes *processes.Manager
	gateway   *gateway.Gateway
}

type Config struct {
	Token     string
	Store     store.Store
	Instances *instances.Manager
	Bus       *eventbus.Bus
	Broker    *permission.Broker
	Loop      *loop.Loop
	Processes *processes.Manager
	Gateway   *gateway.Gateway
}

func New(cfg Config) *Server {
	return &Server{
		token:     cfg.Token,
		store:     cfg.Store,
		instances: cfg.Instances,
		bus:       cfg.Bus,
		broker:    cfg.Broker,
		loop:      cfg.Loop,
		processes: cfg.Processes,
		gateway:   cfg.Gateway,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /instances", s.listInstances)
	mux.HandleFunc("POST /instances", s.startInstance)
	mux.HandleFunc("GET /instances/{id}", s.getInstance)
	mux.HandleFunc("DELETE /instances/{id}", s.stopInstance)
	mux.HandleFunc("GET /instances/{id}/models", s.instanceModels)
	mux.HandleFunc("GET /instances/{id}/sessions", s.listSessions)
	mux.HandleFunc("POST /instances/{id}/sessions", s.startSession)
	mux.HandleFunc("GET /instances/{id}/statistics", s.instanceStatistics)
	mux.HandleFunc("GET /sessions/{id}", s.getSession)
	mux.HandleFunc("POST /sessions/{id}/messages", s.sendMessage)
	mux.HandleFunc("GET /sessions/{id}/agents", s.sessionAgents)
	mux.HandleFunc("GET /sessions/{id}/statistics", s.sessionStatistics)
	mux.HandleFunc("GET /sessions/{id}/processes", s.sessionProcesses)
	mux.HandleFunc("GET /sessions/{id}/events", s.sessionEvents)
	mux.HandleFunc("GET /processes/{id}/output", s.processOutput)
	mux.HandleFunc("POST /permissions/{id}", s.resolvePermission)
	mux.HandleFunc("GET /statistics", s.statistics)
	mux.HandleFunc("GET /providers", s.listProviders)
	mux.HandleFunc("GET /providers/{id}/models", s.providerModels)
	mux.HandleFunc("POST /providers/{id}/refresh", s.refreshProvider)
	return s.auth(mux)
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.token == "" || r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		header := r.Header.Get("Authorization")
		token := strings.TrimPrefix(header, "Bearer ")
		if token == header {
			token = r.URL.Query().Get("token")
		}
		if token != s.token {
			writeError(w, http.StatusUnauthorized, "the token is not correct")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type instanceRequest struct {
	Workspace       string   `json:"workspace"`
	Models          []string `json:"models"`
	DefaultModel    string   `json:"default_model"`
	ProcessLimit    int      `json:"process_limit"`
	AgentDepthLimit int      `json:"agent_depth_limit"`
}

func (s *Server) startInstance(w http.ResponseWriter, r *http.Request) {
	var input instanceRequest
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	spec := atom.InstanceSpec{
		Workspace:       input.Workspace,
		Models:          input.Models,
		DefaultModel:    input.DefaultModel,
		ProcessLimit:    input.ProcessLimit,
		AgentDepthLimit: input.AgentDepthLimit,
		CreatedAt:       time.Now(),
	}
	instance, err := s.instances.Start(r.Context(), spec)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.Instances().Save(r.Context(), instance.Spec()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, instance.Spec())
}

func (s *Server) listInstances(w http.ResponseWriter, r *http.Request) {
	instances, err := s.store.Instances().All(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, instances)
}

func (s *Server) getInstance(w http.ResponseWriter, r *http.Request) {
	instance, ok := s.instances.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "the instance is not in the manager")
		return
	}
	writeJSON(w, http.StatusOK, instance.Spec())
}

func (s *Server) stopInstance(w http.ResponseWriter, r *http.Request) {
	if err := s.instances.Stop(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err := s.store.Instances().Delete(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (s *Server) instanceModels(w http.ResponseWriter, r *http.Request) {
	instance, ok := s.instances.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "the instance is not in the manager")
		return
	}
	writeJSON(w, http.StatusOK, s.modelsOf(instance))
}

func (s *Server) modelsOf(instance *instances.Instance) []atom.ModelInfo {
	allowed := map[string]bool{}
	for _, id := range instance.Spec().Models {
		allowed[id] = true
	}
	var result []atom.ModelInfo
	for _, provider := range s.gateway.Providers() {
		for _, model := range provider.Models() {
			if len(allowed) > 0 && !allowed[model.ID] {
				continue
			}
			result = append(result, model)
		}
	}
	return result
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	instanceID := r.PathValue("id")
	instance, ok := s.instances.Get(instanceID)
	if !ok {
		writeError(w, http.StatusNotFound, "the instance is not in the manager")
		return
	}
	writeJSON(w, http.StatusOK, s.sessionsOf(r.Context(), instance))
}

func (s *Server) sessionsOf(ctx context.Context, instance *instances.Instance) []atom.Session {
	ids, _ := instance.Sessions().Agents(ctx, "")
	all, err := s.store.Instances().All(ctx)
	if err != nil {
		_ = all
	}
	var sessions []atom.Session
	seen := map[atom.SessionID]bool{}
	for _, id := range ids {
		session, ok := instance.Sessions().Get(ctx, id)
		if ok {
			sessions = append(sessions, session)
			seen[id] = true
		}
	}
	_ = seen
	return sessions
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request) {
	instance, ok := s.instances.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "the instance is not in the manager")
		return
	}
	var input struct {
		Model string `json:"model"`
	}
	_ = readJSON(r, &input)
	model := input.Model
	if model == "" {
		model = instance.Spec().DefaultModel
	}
	session := atom.Session{
		ID:         atom.SessionID(newID()),
		InstanceID: instance.ID(),
		Model:      model,
		CreatedAt:  time.Now(),
	}
	if err := s.store.Sessions().Save(r.Context(), session); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, session)
}

func (s *Server) getSession(w http.ResponseWriter, r *http.Request) {
	session, err := s.store.Sessions().Get(r.Context(), atom.SessionID(r.PathValue("id")))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) sessionAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.store.Sessions().Agents(r.Context(), atom.SessionID(r.PathValue("id")))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if agents == nil {
		agents = []atom.SessionID{}
	}
	writeJSON(w, http.StatusOK, agents)
}

func (s *Server) sendMessage(w http.ResponseWriter, r *http.Request) {
	session, err := s.store.Sessions().Get(r.Context(), atom.SessionID(r.PathValue("id")))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	var input struct {
		Content string `json:"content"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	message := atom.Message{
		ID:        newID(),
		SessionID: session.ID,
		Role:      atom.RoleUser,
		Content:   []atom.Content{{Type: atom.Text, Text: input.Content}},
		CreatedAt: time.Now(),
	}
	if err := s.store.Sessions().Append(r.Context(), message); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.loop.Run(r.Context(), session); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	messages, err := s.store.Sessions().Messages(r.Context(), session.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	answer := lastAssistant(messages)
	writeJSON(w, http.StatusOK, map[string]any{"session": session.ID, "message": answer})
}

func lastAssistant(messages []atom.Message) *atom.Message {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == atom.RoleAssistant {
			return &messages[index]
		}
	}
	return nil
}

func (s *Server) sessionStatistics(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.Usage().Session(r.Context(), atom.SessionID(r.PathValue("id")))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) instanceStatistics(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.Usage().Instance(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) statistics(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.Usage().All(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) sessionProcesses(w http.ResponseWriter, r *http.Request) {
	records, err := s.store.Processes().List(r.Context(), atom.SessionID(r.PathValue("id")))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if records == nil {
		records = []atom.ProcessRecord{}
	}
	writeJSON(w, http.StatusOK, records)
}

func (s *Server) resolvePermission(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Kind  atom.VerdictKind `json:"kind"`
		Scope atom.Scope       `json:"scope"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	decision := atom.PermissionDecision{
		Kind:      input.Kind,
		Scope:     input.Scope,
		CreatedAt: time.Now(),
	}
	if !s.broker.Resolve(r.PathValue("id"), decision) {
		writeError(w, http.StatusNotFound, "the permission request is not open")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "resolved"})
}

func (s *Server) listProviders(w http.ResponseWriter, r *http.Request) {
	specs, err := s.store.Providers().All(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if specs == nil {
		specs = []atom.ProviderSpec{}
	}
	writeJSON(w, http.StatusOK, specs)
}

func (s *Server) providerModels(w http.ResponseWriter, r *http.Request) {
	models, err := s.store.Providers().Models(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if models == nil {
		models = []atom.ModelInfo{}
	}
	writeJSON(w, http.StatusOK, models)
}

func (s *Server) refreshProvider(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("id")
	models, err := s.gateway.Refresh(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if err := s.store.Providers().SaveModels(r.Context(), name, models); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, models)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func readJSON(r *http.Request, value any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(value)
}

func newID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(data[:])
}
