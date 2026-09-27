package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func TestStoreRoundTrip(t *testing.T) {
	dsn := os.Getenv("MTT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MTT_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	database, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	id := time.Now().Format("150405.000000")
	instance := atom.InstanceSpec{ID: "instance-" + id, Workspace: "/tmp/work", DefaultModel: "m1", ProcessLimit: 4, AgentDepthLimit: 1, CreatedAt: time.Now()}
	if err := database.Instances().Save(ctx, instance); err != nil {
		t.Fatal(err)
	}
	got, err := database.Instances().Get(ctx, instance.ID)
	if err != nil || got.Workspace != instance.Workspace {
		t.Fatalf("instance = %+v err = %v", got, err)
	}

	parent := atom.Session{ID: atom.SessionID("parent-" + id), InstanceID: instance.ID, Model: "m1", CreatedAt: time.Now()}
	child := atom.Session{ID: atom.SessionID("child-" + id), InstanceID: instance.ID, Parent: parent.ID, Depth: 1, CreatedAt: time.Now()}
	if err := database.Sessions().Save(ctx, parent); err != nil {
		t.Fatal(err)
	}
	if err := database.Sessions().Save(ctx, child); err != nil {
		t.Fatal(err)
	}
	message := atom.Message{
		ID:        "message-" + id,
		SessionID: parent.ID,
		Role:      atom.RoleUser,
		Content:   []atom.Content{{Type: atom.Text, Text: "hello"}},
		CreatedAt: time.Now(),
	}
	if err := database.Sessions().Append(ctx, message); err != nil {
		t.Fatal(err)
	}
	messages, err := database.Sessions().Messages(ctx, parent.ID)
	if err != nil || len(messages) != 1 || messages[0].Content[0].Text != "hello" {
		t.Fatalf("messages = %+v err = %v", messages, err)
	}

	cost := &atom.Cost{Currency: "USD", Value: 0.25}
	if err := database.Usage().Save(ctx, atom.UsageRecord{InstanceID: instance.ID, SessionID: parent.ID, ModelID: "m1", Usage: atom.Usage{Input: 100, Output: 10}, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := database.Usage().Save(ctx, atom.UsageRecord{InstanceID: instance.ID, SessionID: child.ID, ModelID: "m1", Usage: atom.Usage{Input: 50, Cost: cost}, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	stats, err := database.Usage().Session(ctx, parent.ID)
	if err != nil || stats.Calls != 2 || stats.Input != 150 {
		t.Fatalf("session statistics = %+v err = %v", stats, err)
	}
	if stats.Cost == nil || stats.Cost.Value != 0.25 {
		t.Fatalf("session cost = %+v", stats.Cost)
	}

	event := atom.Event{InstanceID: instance.ID, SessionID: parent.ID, Name: atom.EventTurnStart, Payload: []byte(`{}`), Time: time.Now()}
	if err := database.Events().Append(ctx, event); err != nil {
		t.Fatal(err)
	}
	events, err := database.Events().Since(ctx, instance.ID, 0)
	if err != nil || len(events) == 0 {
		t.Fatalf("events = %d err = %v", len(events), err)
	}

	spec := atom.ProviderSpec{Name: "provider-" + id, APIURL: "http://x", ModelListURL: "http://x/models", Secret: "s", Interval: time.Hour}
	if err := database.Providers().Save(ctx, spec); err != nil {
		t.Fatal(err)
	}
	models := []atom.ModelInfo{{ID: "m1", Prices: &atom.Prices{Currency: "USD", Input: 1}}}
	if err := database.Providers().SaveModels(ctx, spec.Name, models); err != nil {
		t.Fatal(err)
	}
	gotModels, err := database.Providers().Models(ctx, spec.Name)
	if err != nil || len(gotModels) != 1 || gotModels[0].Prices == nil {
		t.Fatalf("models = %+v err = %v", gotModels, err)
	}

	record := atom.ProcessRecord{ID: "process-" + id, InstanceID: instance.ID, SessionID: parent.ID, Spec: atom.ProcessSpec{Command: "ls"}, PID: 10, Status: "running", StartedAt: time.Now()}
	if err := database.Processes().Save(ctx, record); err != nil {
		t.Fatal(err)
	}
	gotRecords, err := database.Processes().List(ctx, parent.ID)
	if err != nil || len(gotRecords) != 1 {
		t.Fatalf("processes = %+v err = %v", gotRecords, err)
	}

	decision := atom.PermissionDecision{RequestID: "permission-" + id, Kind: atom.VerdictAllow, Scope: atom.ScopeSession, CreatedAt: time.Now()}
	if err := database.Permissions().Save(ctx, decision); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Permissions().Get(ctx, decision.RequestID); err != nil {
		t.Fatal(err)
	}
}
