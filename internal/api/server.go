package api

import (
	"net/http"

	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/eventbus"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/permission"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/processes"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/providerauth"
)

type Server struct {
	token        string
	store        store.Store
	instances    *instances.Manager
	bus          *eventbus.Bus
	broker       *permission.Broker
	loop         *loop.Loop
	queue        *loop.Queue
	processes    *processes.Manager
	gateway      *gateway.Gateway
	providerAuth *providerauth.Service
}

type Config struct {
	Token        string
	Store        store.Store
	Instances    *instances.Manager
	Bus          *eventbus.Bus
	Broker       *permission.Broker
	Loop         *loop.Loop
	Queue        *loop.Queue
	Processes    *processes.Manager
	Gateway      *gateway.Gateway
	ProviderAuth *providerauth.Service
}

func New(configuration Config) *Server {
	if configuration.Bus != nil && configuration.Store != nil {
		configuration.Bus.SetRecorder(configuration.Store.Events().Record)
	}
	return &Server{
		token:        configuration.Token,
		store:        configuration.Store,
		instances:    configuration.Instances,
		bus:          configuration.Bus,
		broker:       configuration.Broker,
		loop:         configuration.Loop,
		queue:        configuration.Queue,
		processes:    configuration.Processes,
		gateway:      configuration.Gateway,
		providerAuth: configuration.ProviderAuth,
	}
}

func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", server.health)
	mux.HandleFunc("GET /instances", server.listInstances)
	mux.HandleFunc("POST /instances", server.startInstance)
	mux.HandleFunc("GET /instances/{id}", server.getInstance)
	mux.HandleFunc("DELETE /instances/{id}", server.stopInstance)
	mux.HandleFunc("POST /instances/{id}/start", server.resumeInstance)
	mux.HandleFunc("GET /instances/{id}/models", server.instanceModels)
	mux.HandleFunc("GET /instances/{id}/sessions", server.listSessions)
	mux.HandleFunc("POST /instances/{id}/sessions", server.startSession)
	mux.HandleFunc("GET /instances/{id}/statistics", server.instanceStatistics)
	mux.HandleFunc("GET /sessions/{id}", server.getSession)
	mux.HandleFunc("GET /sessions/{id}/messages", server.sessionMessages)
	mux.HandleFunc("POST /sessions/{id}/messages", server.sendMessage)
	mux.HandleFunc("POST /sessions/{id}/cancel", server.cancelMessage)
	mux.HandleFunc("DELETE /sessions/{id}/queue/{message_id}", server.cancelQueuedMessage)
	mux.HandleFunc("GET /sessions/{id}/status", server.sessionStatus)
	mux.HandleFunc("POST /sessions/{id}/revert", server.revertSession)
	mux.HandleFunc("GET /sessions/{id}/agents", server.sessionAgents)
	mux.HandleFunc("GET /sessions/{id}/statistics", server.sessionStatistics)
	mux.HandleFunc("GET /sessions/{id}/processes", server.sessionProcesses)
	mux.HandleFunc("GET /sessions/{id}/events", server.sessionEvents)
	mux.HandleFunc("GET /processes/{id}/output", server.processOutput)
	mux.HandleFunc("POST /permissions/{id}", server.resolvePermission)
	mux.HandleFunc("GET /statistics", server.statistics)
	mux.HandleFunc("GET /providers", server.listProviders)
	mux.HandleFunc("GET /providers/{id}/models", server.providerModels)
	mux.HandleFunc("POST /providers/{id}/refresh", server.refreshProvider)
	mux.HandleFunc("PUT /providers/{id}/key", server.saveProviderKey)
	mux.HandleFunc("DELETE /providers/{id}/key", server.deleteProviderKey)
	mux.HandleFunc("POST /providers/{id}/auth/device", server.startProviderLogin)
	mux.HandleFunc("GET /providers/{id}/auth/device/{login_id}", server.providerLoginStatus)
	mux.HandleFunc("DELETE /providers/{id}/auth/device/{login_id}", server.cancelProviderLogin)
	mux.HandleFunc("DELETE /providers/{id}/auth", server.disconnectProvider)
	mux.HandleFunc("GET /settings", server.globalSettings)
	mux.HandleFunc("PUT /settings/{key}", server.saveGlobalSetting)
	mux.HandleFunc("DELETE /settings/{key}", server.deleteGlobalSetting)
	mux.HandleFunc("GET /instances/{id}/settings", server.instanceSettings)
	mux.HandleFunc("PUT /instances/{id}/settings/{key}", server.saveInstanceSetting)
	mux.HandleFunc("DELETE /instances/{id}/settings/{key}", server.deleteInstanceSetting)
	mux.HandleFunc("PUT /instances/{id}/providers/{provider}/key", server.saveInstanceKey)
	mux.HandleFunc("DELETE /instances/{id}/providers/{provider}/key", server.deleteInstanceKey)
	return server.authenticateAPIRequest(mux)
}
