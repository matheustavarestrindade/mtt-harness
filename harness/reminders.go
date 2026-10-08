package harness

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

// ReminderDelivery counts request input added by an accepted contribution.
// It is not a separate provider usage record or an additional billed cost.
type ReminderDelivery struct {
	Tokens    int
	Estimated bool
}

// ReminderContribution contains only request-local messages. Commit runs in the
// turn worker immediately before provider dispatch, after all budget checks.
// A failed dispatch can replay the same durable reminder on the next request.
type ReminderContribution struct {
	Messages []atom.Message
	Commit   func(context.Context, ReminderDelivery) error
	Deferred func(context.Context) error
}

// RequestReminderPlugin augments, but never selects or deletes, conversation
// content. ProjectReminders must be pure against state frozen by BeginRequest:
// context fitting can call it repeatedly on different candidate views.
type RequestReminderPlugin interface {
	ProjectReminders(context.Context, atom.Session, atom.Request) (atom.Request, error)
	PrepareReminder(context.Context, ContextRequest) (ReminderContribution, error)
}

// ReminderPlan belongs to one provider request and is committed at dispatch.
type ReminderPlan struct {
	Request atom.Request
	Commit  func(context.Context) error
}

// RequestReminderRuntime is an optional host capability. Existing plugin
// runtimes need not implement it when they have no request reminders.
type RequestReminderRuntime interface {
	ProjectReminders(context.Context, atom.Session, atom.Request) (atom.Request, error)
	PrepareReminders(context.Context, ContextRequest) (ReminderPlan, error)
}
