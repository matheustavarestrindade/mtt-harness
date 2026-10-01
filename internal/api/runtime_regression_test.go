package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/api"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/eventbus"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/permission"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	sessionmemory "github.com/matheustavarestrindade/mtt-harness/internal/organism/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/registry"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestInstanceIsolationSettingsTokenAndStopResume(test *testing.T) {
	operationContext := context.Background()
	database := memory.New()
	runtime := harness.New()
	manager := instances.New(func(identifier string) instances.SessionManager {
		return sessionmemory.New(identifier, database.Sessions())
	}, database.Settings())
	models := gateway.New(runtime)
	testutil.RequireNoError(test, models.Add(provider.NewTest("test")))
	bus := eventbus.New(runtime)
	broker := permission.NewBroker()
	runner := loop.New(runtime, loop.Config{Gateway: models, Registry: registry.New(runtime, nil), Store: database, Bus: bus, Broker: broker, Engine: permission.NewEngine(), Instances: manager})
	queue := loop.NewQueue(runner)
	defer queue.Close(operationContext)
	server := httptest.NewServer(api.New(api.Config{Token: "initial", Store: database, Instances: manager, Gateway: models, Queue: queue, Loop: runner, Bus: bus, Broker: broker}).Handler())
	defer server.Close()
	var created []atom.InstanceSpec
	for _, name := range []string{"first", "second"} {
		var instance atom.InstanceSpec
		response := request(test, server.URL+"/instances", "initial", "POST", map[string]any{"workspace": test.TempDir(), "default_model": "test-model", "agent_depth_limit": 2})
		if response.StatusCode != http.StatusCreated {
			test.Fatalf("create %s: %d", name, response.StatusCode)
		}
		decode(test, response, &instance)
		created = append(created, instance)
		testutil.RequireNoError(test, database.Sessions().Save(operationContext, atom.Session{ID: atom.SessionID(name), InstanceID: instance.ID, Model: "test-model"}))
	}
	var sessions []atom.Session
	decode(test, request(test, server.URL+"/instances/"+created[0].ID+"/sessions", "initial", "GET", nil), &sessions)
	if len(sessions) != 1 || sessions[0].ID != "first" {
		test.Fatalf("sessions crossed instances: %+v", sessions)
	}
	response := request(test, server.URL+"/instances/"+created[0].ID+"/settings/agent_depth_limit", "initial", "PUT", map[string]string{"value": "5"})
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		test.Fatalf("setting update: %d", response.StatusCode)
	}
	limit, operationError := manager.AgentDepthLimit(operationContext, created[0].ID)
	testutil.RequireNoError(test, operationError)
	if limit != 5 {
		test.Fatalf("creation value overrode new setting: %d", limit)
	}
	response = request(test, server.URL+"/settings/api_token", "initial", "PUT", map[string]string{"value": "rotated"})
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		test.Fatalf("rotate token: %d", response.StatusCode)
	}
	response = request(test, server.URL+"/instances", "initial", "GET", nil)
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		test.Fatal("old token still accepted")
	}
	response = request(test, server.URL+"/instances/"+created[0].ID, "rotated", "DELETE", nil)
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		test.Fatalf("stop: %d", response.StatusCode)
	}
	saved, operationError := database.Instances().Get(operationContext, created[0].ID)
	testutil.RequireNoError(test, operationError)
	if !saved.Stopped {
		test.Fatal("stop did not preserve configuration")
	}
	response = request(test, server.URL+"/sessions/first/messages", "rotated", "POST", map[string]string{"content": "while stopped"})
	response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		test.Fatalf("stopped session accepted input: %d", response.StatusCode)
	}
	response = request(test, server.URL+"/instances/"+created[0].ID+"/start", "rotated", "POST", nil)
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		test.Fatalf("resume: %d", response.StatusCode)
	}
	if !manager.IsRunning(created[0].ID) {
		test.Fatal("instance did not resume")
	}
}
