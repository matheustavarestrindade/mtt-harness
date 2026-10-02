package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/api"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestSessionReasoningUsesAdvertisedChoicesAndPreservesLifecycleState(test *testing.T) {
	operationContext := context.Background()
	database := memory.New()
	harnessRuntime := harness.New()
	modelGateway := gateway.New(harnessRuntime)
	modelProvider := provider.New(atom.ProviderSpec{Name: "fixture"})
	modelProvider.SetModels([]atom.ModelInfo{{ID: "model", Reasoning: true, ReasoningEfforts: []string{"low", "high"}, Input: []atom.MediaType{atom.Text}}})
	testutil.RequireNoError(test, modelGateway.Add(modelProvider))
	instanceManager := instances.New(func(string) instances.SessionManager { return nil }, nil)
	_, operationError := instanceManager.Start(operationContext, atom.InstanceSpec{ID: "instance", Workspace: test.TempDir(), DefaultModel: "fixture/model"})
	testutil.RequireNoError(test, operationError)
	server := httptest.NewServer(api.New(api.Config{Token: "secret", Store: database, Instances: instanceManager, Gateway: modelGateway}).Handler())
	defer server.Close()
	response := request(test, server.URL+"/instances/instance/sessions", "secret", "POST", map[string]any{"reasoning_effort": "high"})
	if response.StatusCode != http.StatusCreated {
		test.Fatalf("create status: %d", response.StatusCode)
	}
	var session atom.Session
	testutil.RequireNoError(test, json.NewDecoder(response.Body).Decode(&session))
	response.Body.Close()
	if session.ReasoningEffort != "high" {
		test.Fatal("creation lost selected effort")
	}
	for _, scenario := range []struct {
		effort string
		status int
	}{{"none", http.StatusBadRequest}, {"low", http.StatusOK}, {"", http.StatusOK}} {
		response = request(test, server.URL+"/sessions/"+string(session.ID)+"/reasoning", "secret", "PUT", map[string]any{"effort": scenario.effort})
		response.Body.Close()
		if response.StatusCode != scenario.status {
			test.Fatalf("effort %q: status %d", scenario.effort, response.StatusCode)
		}
	}
	// A coordinator can still hold the original session while it finishes work.
	session.Completed = true
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	saved, operationError := database.Sessions().Get(operationContext, session.ID)
	testutil.RequireNoError(test, operationError)
	if !saved.Completed || saved.ReasoningEffort != "" {
		test.Fatalf("lifecycle save overwrote effort: %+v", saved)
	}
	response = request(test, server.URL+"/sessions/"+string(session.ID)+"/reasoning", "secret", "PUT", map[string]any{"effort": "high"})
	response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		test.Fatal("completed child accepted a setting change")
	}
}
