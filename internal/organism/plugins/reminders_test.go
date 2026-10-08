package plugins

import (
	"context"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

type reminderTestPlugin struct {
	committed bool
	deferred  bool
	mutate    bool
}

func (plugin *reminderTestPlugin) Name() string                 { return "fixture" }
func (plugin *reminderTestPlugin) Version() string              { return "1" }
func (plugin *reminderTestPlugin) Setup(*harness.Harness) error { return nil }
func (plugin *reminderTestPlugin) ProjectReminders(operationContext context.Context, session atom.Session, request atom.Request) (atom.Request, error) {
	if plugin.mutate {
		request.Messages[0].Content[0].Text = "changed"
	}
	return request, nil
}
func (plugin *reminderTestPlugin) PrepareReminder(context.Context, harness.ContextRequest) (harness.ReminderContribution, error) {
	return harness.ReminderContribution{Messages: []atom.Message{{ID: "fixture:1", Role: atom.RoleSystem, Ephemeral: true, InContext: true, Content: []atom.Content{{Type: atom.Text, Text: "reminder"}}}}, Commit: func(context.Context, harness.ReminderDelivery) error { plugin.committed = true; return nil }, Deferred: func(context.Context) error { plugin.deferred = true; return nil }}, nil
}

func TestReminderPlanHonorsPolicyCeilingAndCommitsOnlyAtDispatch(test *testing.T) {
	host := New(harness.New())
	plugin := &reminderTestPlugin{}
	testutil.RequireNoError(test, host.Attach(plugin))
	measure := func(operationContext context.Context, request atom.Request) (harness.RequestBudget, error) {
		return harness.RequestBudget{InputTokens: len(request.Messages) * 10, InputLimit: 100}, nil
	}
	input := harness.ContextRequest{Session: atom.Session{ID: "session"}, Request: atom.Request{Messages: []atom.Message{{ID: "user", Role: atom.RoleUser}}}, Measure: measure, InputCeiling: 15}
	plan, operationError := host.PrepareReminders(context.Background(), input)
	testutil.RequireNoError(test, operationError)
	if len(plan.Request.Messages) != 1 || !plugin.deferred || plugin.committed {
		test.Fatal("reminder exceeded the policy ceiling or committed early")
	}
	input.InputCeiling = 50
	plugin.deferred = false
	plan, operationError = host.PrepareReminders(context.Background(), input)
	testutil.RequireNoError(test, operationError)
	if len(plan.Request.Messages) != 2 || plugin.committed || plugin.deferred {
		test.Fatal("valid reminder was not staged")
	}
	testutil.RequireNoError(test, plan.Commit(context.Background()))
	if !plugin.committed {
		test.Fatal("dispatch did not commit")
	}
}

func TestReminderProjectionRejectsMutationOfExistingText(test *testing.T) {
	host := New(harness.New())
	testutil.RequireNoError(test, host.Attach(&reminderTestPlugin{mutate: true}))
	_, operationError := host.ProjectReminders(context.Background(), atom.Session{}, atom.Request{Messages: []atom.Message{{ID: "user", Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "original"}}}}})
	if operationError == nil {
		test.Fatal("reminder plugin changed user content")
	}
}
