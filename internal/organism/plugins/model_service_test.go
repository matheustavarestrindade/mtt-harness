package plugins

import (
	"context"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestWorkspaceAgentAccountingDoesNotCreateSessionCosts(test *testing.T) {
	operationContext := context.Background()
	database := memory.New()
	runtime := harness.New()
	modelGateway := gateway.New(runtime)
	testProvider := provider.NewTest("worker", provider.TextWithUsage("result", atom.Usage{Input: 100, Output: 20, CacheRead: 30}), provider.Text("no usage reported"))
	testutil.RequireNoError(test, modelGateway.Add(testProvider))
	manager := instances.New(func(string) instances.SessionManager { return nil }, database.Settings())
	instance, operationError := manager.Start(operationContext, atom.InstanceSpec{ID: "workspace", Workspace: test.TempDir(), Models: []string{"worker/worker-model"}})
	testutil.RequireNoError(test, operationError)
	session := atom.Session{ID: "origin", InstanceID: instance.ID()}
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	services := &Services{Gateway: modelGateway, Instances: manager, Store: database}
	request := harness.WorkspaceAgentRequest{WorkspaceID: instance.ID(), Agent: "context.historian", RunID: "maintenance", RequestID: "request-one", SourceSessionID: session.ID, Model: "worker/worker-model", Messages: []atom.Message{{Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "Summarize data."}}}}, MaxOutputTokens: 512}
	response, operationError := services.Run(operationContext, request)
	testutil.RequireNoError(test, operationError)
	if response.Usage.Cost == nil || !response.Usage.Cost.Estimated {
		test.Fatal("background request lost estimated pricing")
	}
	chat, operationError := database.Usage().Session(operationContext, session.ID)
	testutil.RequireNoError(test, operationError)
	if chat.Calls != 0 {
		test.Fatal("background request was charged to its originating chat")
	}
	workspace, operationError := database.Usage().Instance(operationContext, instance.ID())
	testutil.RequireNoError(test, operationError)
	if workspace.Calls != 1 || workspace.Input != 100 {
		test.Fatal("workspace did not record the background request")
	}
	agents, operationError := database.Usage().Agents(operationContext, instance.ID())
	testutil.RequireNoError(test, operationError)
	if len(agents) != 1 || agents[0].Agent != "context.historian" || agents[0].Statistics.CacheRead != 30 {
		test.Fatal("agent usage breakdown is incorrect")
	}
	request.RequestID = "request-two"
	response, operationError = services.Run(operationContext, request)
	testutil.RequireNoError(test, operationError)
	if response.Usage.Cost != nil {
		test.Fatal("missing provider usage was converted to known zero cost")
	}
	request.Model = "other/model"
	request.RequestID = "rejected"
	if _, operationError := services.Run(operationContext, request); operationError == nil {
		test.Fatal("worker bypassed model availability")
	}
	if len(testProvider.Requests) != 2 || len(testProvider.Requests[0].Tools) != 0 {
		test.Fatal("background service exposed tools or sent a rejected request")
	}
}

type mutatingContextPlugin struct{}

func (mutatingContextPlugin) Name() string                 { return "mutating" }
func (mutatingContextPlugin) Version() string              { return "test" }
func (mutatingContextPlugin) Setup(*harness.Harness) error { return nil }
func (mutatingContextPlugin) PrepareContext(_ context.Context, input harness.ContextRequest) (harness.ContextSelection, error) {
	input.Request.Params["temperature"] = 9
	return harness.ContextSelection{Request: input.Request}, nil
}

func TestContextPolicyCannotMutateOtherRequestFieldsInPlace(test *testing.T) {
	host := New(harness.New())
	testutil.RequireNoError(test, host.Attach(mutatingContextPlugin{}))
	_, operationError := host.PrepareContext(context.Background(), harness.ContextRequest{Request: atom.Request{Model: "fixture", Params: map[string]any{"temperature": 1}}, Measure: func(context.Context, atom.Request) (harness.RequestBudget, error) {
		return harness.RequestBudget{}, nil
	}})
	if operationError == nil {
		test.Fatal("shared-map mutation bypassed context policy restrictions")
	}
}
