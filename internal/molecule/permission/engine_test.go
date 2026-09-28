package permission

import (
	"context"
	"errors"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestPermissionScopesAndRestart(test *testing.T) {
	database := memory.New()
	engine := NewEngine()
	engine.SetStore(database.Permissions())
	session := atom.Session{ID: "one", InstanceID: "workspace"}
	operationContext := context.Background()
	for _, scope := range []atom.Scope{atom.ScopeOnce, atom.ScopeSession, atom.ScopeAlways} {
		testutil.RequireNoError(test, engine.Remember(operationContext, session, string(scope), atom.PermissionDecision{RequestID: string(scope), Kind: atom.VerdictAllow, Scope: scope}))
	}
	restored := NewEngine()
	restored.SetStore(database.Permissions())
	for _, scenario := range []struct {
		session  atom.Session
		target   string
		expected bool
	}{
		{session, "once", false}, {session, "session", true},
		{atom.Session{ID: "two", InstanceID: "workspace"}, "session", false},
		{atom.Session{ID: "two", InstanceID: "workspace"}, "always", true},
		{atom.Session{ID: "one", InstanceID: "other"}, "always", false},
	} {
		_, found, operationError := restored.Cached(operationContext, scenario.session, scenario.target)
		testutil.RequireNoError(test, operationError)
		if found != scenario.expected {
			test.Fatalf("scope %s in %+v: found=%t", scenario.target, scenario.session, found)
		}
	}
}

func TestBrokerCancellationRemovesRequest(test *testing.T) {
	broker := NewBroker()
	operationContext, cancel := context.WithCancel(context.Background())
	cancel()
	_, operationError := broker.Request(operationContext, atom.PermissionRequest{ID: "cancelled"})
	if !errors.Is(operationError, context.Canceled) || broker.Pending() != 0 {
		test.Fatalf("cancelled request remains: %v pending=%d", operationError, broker.Pending())
	}
	if broker.Resolve("cancelled", atom.PermissionDecision{Kind: atom.VerdictAllow, Scope: atom.ScopeAlways}) {
		test.Fatal("cancelled request accepted a late approval")
	}
}
