package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestStoreRoundTrip(test *testing.T) {
	databaseURL := os.Getenv("MTT_TEST_DATABASE_URL")
	if databaseURL == "" {
		test.Skip("MTT_TEST_DATABASE_URL is not set")
	}
	operationContext := context.Background()
	database, operationError := Open(operationContext, databaseURL)
	testutil.RequireNoError(test, operationError)

	defer database.Close()

	identifier := time.Now().Format("150405.000000")
	instance := atom.InstanceSpec{ID: "instance-" + identifier, Workspace: "/tmp/work", DefaultModel: "m1", ProcessLimit: 4, AgentDepthLimit: 1, CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Instances().Save(operationContext, instance))

	actual, operationError := database.Instances().Get(operationContext, instance.ID)
	if operationError != nil || actual.Workspace != instance.Workspace {
		test.Fatalf("instance = %+v err = %v", actual, operationError)
	}

	parent := atom.Session{ID: atom.SessionID("parent-" + identifier), InstanceID: instance.ID, Model: "m1", CreatedAt: time.Now()}
	child := atom.Session{ID: atom.SessionID("child-" + identifier), InstanceID: instance.ID, Parent: parent.ID, Depth: 1, CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, parent))
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, child))

	message := atom.Message{
		ID:        "message-" + identifier,
		SessionID: parent.ID,
		Role:      atom.RoleUser,
		Content:   []atom.Content{{Type: atom.Text, Text: "hello"}},
		CreatedAt: time.Now(),
	}
	testutil.RequireNoError(test, database.Sessions().Append(operationContext, message))

	messages, operationError := database.Sessions().Messages(operationContext, parent.ID)
	if operationError != nil || len(messages) != 1 || messages[0].Content[0].Text != "hello" {
		test.Fatalf("messages = %+v err = %v", messages, operationError)
	}

	cost := &atom.Cost{Currency: "USD", Value: 0.25}
	testutil.RequireNoError(test, database.Usage().Save(operationContext, atom.UsageRecord{InstanceID: instance.ID, SessionID: parent.ID, ModelID: "m1", Usage: atom.Usage{Input: 100, Output: 10}, CreatedAt: time.Now()}))
	testutil.RequireNoError(test, database.Usage().Save(operationContext, atom.UsageRecord{InstanceID: instance.ID, SessionID: child.ID, ModelID: "m1", Usage: atom.Usage{Input: 50, Cost: cost}, CreatedAt: time.Now()}))

	statistics, operationError := database.Usage().Session(operationContext, parent.ID)
	if operationError != nil || statistics.Calls != 2 || statistics.Input != 150 {
		test.Fatalf("session statistics = %+v err = %v", statistics, operationError)
	}
	if statistics.Cost == nil || statistics.Cost.Value != 0.25 {
		test.Fatalf("session cost = %+v", statistics.Cost)
	}

	event := atom.Event{InstanceID: instance.ID, SessionID: parent.ID, Name: atom.EventTurnStart, Payload: []byte(`{}`), Time: time.Now()}
	testutil.RequireNoError(test, database.Events().Append(operationContext, event))

	events, operationError := database.Events().Since(operationContext, instance.ID, 0)
	if operationError != nil || len(events) == 0 {
		test.Fatalf("events = %d err = %v", len(events), operationError)
	}

	providerSpec := atom.ProviderSpec{Name: "provider-" + identifier, APIURL: "http://x", ModelListURL: "http://x/models", Interval: time.Hour}
	testutil.RequireNoError(test, database.Providers().Save(operationContext, providerSpec))

	models := []atom.ModelInfo{{ID: "m1", Prices: &atom.Prices{Currency: "USD", Input: 1}}}
	testutil.RequireNoError(test, database.Providers().SaveModels(operationContext, providerSpec.Name, models))

	gotModels, operationError := database.Providers().Models(operationContext, providerSpec.Name)
	if operationError != nil || len(gotModels) != 1 || gotModels[0].Prices == nil {
		test.Fatalf("models = %+v err = %v", gotModels, operationError)
	}

	record := atom.ProcessRecord{ID: "process-" + identifier, InstanceID: instance.ID, SessionID: parent.ID, Spec: atom.ProcessSpec{Command: "ls"}, PID: 10, Status: "running", StartedAt: time.Now()}
	testutil.RequireNoError(test, database.Processes().Save(operationContext, record))

	gotRecords, operationError := database.Processes().List(operationContext, parent.ID)
	if operationError != nil || len(gotRecords) != 1 {
		test.Fatalf("processes = %+v err = %v", gotRecords, operationError)
	}

	decision := atom.PermissionDecision{RequestID: "permission-" + identifier, Kind: atom.VerdictAllow, Scope: atom.ScopeSession, CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Permissions().Save(operationContext, decision))

	if _, operationError := database.Permissions().Get(operationContext, decision.RequestID); operationError != nil {
		test.Fatal(operationError)
	}
}
