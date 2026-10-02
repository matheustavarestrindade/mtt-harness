package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestReasoningAndPricingSurvivePostgresRoundTrip(test *testing.T) {
	database := regressionStore(test)
	operationContext := context.Background()
	identifier := fmt.Sprintf("reasoning-%d", time.Now().UnixNano())
	session := atom.Session{ID: atom.SessionID(identifier), InstanceID: identifier, Model: "fixture/model", ReasoningEffort: "low", CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	testutil.RequireNoError(test, database.Sessions().SetReasoningEffort(operationContext, session.ID, "high"))
	session.Completed = true
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	loaded, operationError := database.Sessions().Get(operationContext, session.ID)
	testutil.RequireNoError(test, operationError)
	if loaded.ReasoningEffort != "high" || !loaded.Completed {
		test.Fatalf("session setting was lost: %+v", loaded)
	}
	message := atom.Message{ID: identifier, SessionID: session.ID, Role: atom.RoleAssistant, Reasoning: "Visible thinking", Content: []atom.Content{{Type: atom.Text, Text: "Answer"}}, ProviderState: &atom.ProviderState{Provider: "fixture", ChatReasoning: "private continuation"}, CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Sessions().Append(operationContext, message))
	messages, operationError := database.Sessions().Messages(operationContext, session.ID)
	testutil.RequireNoError(test, operationError)
	public, operationError := json.Marshal(messages)
	testutil.RequireNoError(test, operationError)
	if len(messages) != 1 || messages[0].Reasoning != message.Reasoning || messages[0].ProviderState.ChatReasoning != "private continuation" || strings.Contains(string(public), "private continuation") {
		test.Fatal("reasoning persistence or public/private separation failed")
	}
	specification := atom.ProviderSpec{Name: identifier, MetadataURL: "https://example.test/catalog.json", MetadataFormat: "models_dev", MetadataProvider: "fixture", Billing: "tokens"}
	testutil.RequireNoError(test, database.Providers().Save(operationContext, specification))
	savedSpec, operationError := database.Providers().Get(operationContext, identifier)
	testutil.RequireNoError(test, operationError)
	if savedSpec.MetadataURL != specification.MetadataURL || savedSpec.MetadataProvider != "fixture" || savedSpec.Billing != "tokens" {
		test.Fatal("metadata source configuration was lost")
	}
	model := atom.ModelInfo{ID: "model", Reasoning: true, ReasoningEfforts: []string{"low", "high"}, DefaultReasoningEffort: "low", ReasoningSummary: "auto", Billing: "tokens", Prices: &atom.Prices{Currency: "USD", Input: 1, Output: 3, CacheWriteUnknown: true, Tiers: []atom.PriceTier{{AboveInputTokens: 100, Input: 2, Output: 5}}}}
	testutil.RequireNoError(test, database.Providers().SaveModels(operationContext, identifier, []atom.ModelInfo{model}))
	models, operationError := database.Providers().Models(operationContext, identifier)
	testutil.RequireNoError(test, operationError)
	if len(models) != 1 || !models[0].Reasoning || strings.Join(models[0].ReasoningEfforts, ",") != "low,high" || models[0].ReasoningSummary != "auto" || len(models[0].Prices.Tiers) != 1 || !models[0].Prices.CacheWriteUnknown {
		test.Fatalf("model metadata was lost: %+v", models)
	}
	for _, estimated := range []bool{false, true} {
		testutil.RequireNoError(test, database.Usage().Save(operationContext, atom.UsageRecord{SessionID: session.ID, InstanceID: identifier, ModelID: "fixture/model", CreatedAt: time.Now(), Usage: atom.Usage{Cost: &atom.Cost{Currency: "USD", Value: 0.25, Estimated: estimated}}}))
	}
	statistics, operationError := database.Usage().Session(operationContext, session.ID)
	testutil.RequireNoError(test, operationError)
	if statistics.Cost == nil || !statistics.Cost.Estimated || statistics.Cost.Value != 0.5 {
		test.Fatalf("mixed estimates were labeled as exact charges: %+v", statistics)
	}
}
