package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/api"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestSessionModelSwitchRequiresContextConfirmationAndAtomicSelection(test *testing.T) {
	operationContext := context.Background()
	database := memory.New()
	modelGateway := gateway.New(harness.New())
	modelProvider := provider.New(atom.ProviderSpec{Name: "fixture"})
	modelProvider.SetModels([]atom.ModelInfo{
		{ID: "base", ContextMax: 4000, Reasoning: true, ReasoningEfforts: []string{"high"}},
		{ID: "larger", ContextMax: 8000, Reasoning: true, ReasoningEfforts: []string{"high"}},
		{ID: "equal", ContextMax: 8000}, {ID: "smaller", ContextMax: 2000}, {ID: "unknown"},
	})
	testutil.RequireNoError(test, modelGateway.Add(modelProvider))
	instanceManager := instances.New(func(string) instances.SessionManager { return nil }, nil)
	_, operationError := instanceManager.Start(operationContext, atom.InstanceSpec{ID: "instance", Workspace: test.TempDir(), DefaultModel: "fixture/base"})
	testutil.RequireNoError(test, operationError)
	session := atom.Session{ID: "session", InstanceID: "instance", Model: "fixture/base", ReasoningEffort: "high"}
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	testutil.RequireNoError(test, database.Sessions().Append(operationContext, atom.Message{ID: "kept", SessionID: session.ID, Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "Keep history"}}}))
	server := httptest.NewServer(api.New(api.Config{Token: "secret", Store: database, Instances: instanceManager, Gateway: modelGateway}).Handler())
	defer server.Close()
	for _, scenario := range []struct {
		model     string
		confirmed bool
		status    int
	}{
		{"larger", false, 200}, {"equal", false, 200}, {"smaller", false, 409}, {"smaller", true, 200}, {"unknown", true, 400}, {"missing", true, 400},
	} {
		response := request(test, server.URL+"/sessions/session/model", "secret", "PUT", map[string]any{"model": "fixture/" + scenario.model, "allow_compaction": scenario.confirmed})
		var body map[string]any
		testutil.RequireNoError(test, json.NewDecoder(response.Body).Decode(&body))
		response.Body.Close()
		if response.StatusCode != scenario.status {
			test.Fatalf("%s: status %d: %v", scenario.model, response.StatusCode, body)
		}
		if scenario.status == http.StatusConflict && body["code"] != "context_compaction_required" {
			test.Fatalf("missing compaction warning: %v", body)
		}
		if scenario.status == http.StatusConflict {
			current, operationError := database.Sessions().Get(operationContext, session.ID)
			testutil.RequireNoError(test, operationError)
			if current.Model != "fixture/equal" {
				test.Fatal("unconfirmed switch changed the model")
			}
		}
	}
	// A stale reasoning update and an old lifecycle writer must not restore the old model.
	operationError = database.Sessions().SetModelSelection(operationContext, session.ID, session.ModelSelection(), atom.SessionModelSelection{Model: session.Model, ReasoningEffort: "high"})
	if !errors.Is(operationError, store.ErrSessionSelectionChanged) {
		test.Fatal("stale selection update was accepted")
	}
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	current, operationError := database.Sessions().Get(operationContext, session.ID)
	testutil.RequireNoError(test, operationError)
	if current.Model != "fixture/smaller" || current.ReasoningEffort != "" {
		test.Fatalf("model or default effort was lost: %+v", current)
	}
	messages, operationError := database.Sessions().Messages(operationContext, session.ID)
	testutil.RequireNoError(test, operationError)
	if len(messages) != 1 || messages[0].ID != "kept" {
		test.Fatal("switch deleted saved history")
	}
}
