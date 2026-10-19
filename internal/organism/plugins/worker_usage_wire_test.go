package plugins

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestMemoryWorkerCountsProviderInputCacheAndReasoningOnce(test *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(responseWriter, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"worker-response\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"Extracted memory\"}]}],\"usage\":{\"input_tokens\":1000,\"input_tokens_details\":{\"cached_tokens\":600},\"output_tokens\":200,\"output_tokens_details\":{\"reasoning_tokens\":125},\"cost\":0.17}}}\n\n")
	}))
	defer server.Close()
	database := memory.New()
	harnessRuntime := harness.New()
	modelGateway := gateway.New(harnessRuntime)
	modelProvider := provider.New(atom.ProviderSpec{Name: "metered", Protocol: "responses", APIURL: server.URL})
	defer modelProvider.Close()
	modelProvider.SetModels([]atom.ModelInfo{{ID: "worker", ContextMax: 32000}})
	testutil.RequireNoError(test, modelGateway.Add(modelProvider))
	instanceManager := instances.New(func(string) instances.SessionManager { return nil }, database.Settings())
	instance, operationError := instanceManager.Start(context.Background(), atom.InstanceSpec{ID: "workspace", Workspace: test.TempDir(), Models: []string{"metered/worker"}})
	testutil.RequireNoError(test, operationError)
	testutil.RequireNoError(test, database.Sessions().Save(context.Background(), atom.Session{ID: "chat", InstanceID: instance.ID()}))
	services := &Services{Gateway: modelGateway, Instances: instanceManager, Store: database}
	response, operationError := services.Run(context.Background(), harness.WorkspaceAgentRequest{WorkspaceID: instance.ID(), SourceSessionID: "chat", Agent: "context.compactor", RunID: "run", RequestID: "request", Model: "metered/worker", MaxOutputTokens: 512, Messages: []atom.Message{{Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "Extract a memory."}}}}})
	testutil.RequireNoError(test, operationError)
	usage := response.Usage
	if usage.Input != 400 || usage.CacheRead != 600 || usage.Output != 200 || usage.Reasoning != 125 || usage.Cost == nil || usage.Cost.Value != 0.17 {
		test.Fatalf("provider usage changed: %#v", usage)
	}
	agents, operationError := database.Usage().Agents(context.Background(), instance.ID())
	testutil.RequireNoError(test, operationError)
	if len(agents) != 1 {
		test.Fatalf("worker was recorded more than once: %#v", agents)
	}
	statistics := agents[0].Statistics
	if statistics.Calls != 1 || statistics.Input+statistics.CacheRead+statistics.CacheWrite+statistics.Output != 1200 || statistics.Reasoning != 125 {
		test.Fatalf("cache/reasoning was double counted: %#v", statistics)
	}
	chat, operationError := database.Usage().Session(context.Background(), "chat")
	testutil.RequireNoError(test, operationError)
	if chat.Calls != 0 || chat.Input != 0 || chat.Output != 0 {
		test.Fatal("memory worker usage leaked into chat totals")
	}
}
