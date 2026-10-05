package store_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/postgres"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestConversationDeletionStorageContract(test *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		test.Run(backend, func(test *testing.T) {
			operationContext := context.Background()
			var database store.Store = memory.New()
			if backend == "postgres" {
				databaseURL := os.Getenv("MTT_TEST_DATABASE_URL")
				if databaseURL == "" {
					test.Skip("MTT_TEST_DATABASE_URL is not set")
				}
				var operationError error
				database, operationError = postgres.Open(operationContext, databaseURL)
				testutil.RequireNoError(test, operationError)
			}
			defer database.Close()
			prefix := fmt.Sprintf("delete-%s-%d", backend, time.Now().UnixNano())
			root := atom.Session{ID: atom.SessionID(prefix), InstanceID: prefix, Model: "fixture/model", CreatedAt: time.Now()}
			child := atom.Session{ID: root.ID + "-child", Parent: root.ID, InstanceID: prefix, CreatedAt: time.Now()}
			foreign := atom.Session{ID: root.ID + "-foreign", Parent: root.ID, InstanceID: prefix + "-other", CreatedAt: time.Now()}
			for _, session := range []atom.Session{root, child, foreign} {
				testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
			}
			message := atom.Message{ID: prefix + "-message", SessionID: child.ID, Role: atom.RoleAssistant,
				Content: []atom.Content{{Type: atom.Text, Text: "deleted conversation content"}}, Reasoning: "deleted thinking", CreatedAt: time.Now()}
			event := atom.Event{InstanceID: prefix, SessionID: child.ID, Name: "model.chunk", Payload: []byte(`{"text":"deleted event content"}`), Time: time.Now()}
			process := atom.ProcessRecord{ID: prefix + "-process", InstanceID: prefix, SessionID: child.ID, Status: "stopped", StartedAt: time.Now(), EndedAt: time.Now()}
			decision := atom.PermissionDecision{RequestID: prefix + "-permission", InstanceID: prefix, SessionID: child.ID, Scope: atom.ScopeSession, Kind: atom.VerdictAllow, Target: "file", CreatedAt: time.Now()}
			persistentDecision := decision
			persistentDecision.RequestID, persistentDecision.Scope = prefix+"-always", atom.ScopeAlways
			testutil.RequireNoError(test, database.Sessions().Append(operationContext, message))
			testutil.RequireNoError(test, database.Events().Append(operationContext, event))
			testutil.RequireNoError(test, database.Processes().Save(operationContext, process))
			testutil.RequireNoError(test, database.Permissions().Save(operationContext, decision))
			testutil.RequireNoError(test, database.Permissions().Save(operationContext, persistentDecision))
			testutil.RequireNoError(test, database.Usage().Save(operationContext, atom.UsageRecord{SessionID: child.ID, InstanceID: prefix, CreatedAt: time.Now(), Usage: atom.Usage{Input: 17, Cost: &atom.Cost{Currency: "USD", Value: 0.25, Estimated: true}}}))
			deleted, operationError := database.Sessions().DeleteConversation(operationContext, root.ID)
			testutil.RequireNoError(test, operationError)
			if len(deleted) != 2 {
				test.Fatalf("deleted sessions: %v", deleted)
			}
			for _, identifier := range []atom.SessionID{root.ID, child.ID} {
				if _, operationError := database.Sessions().Get(operationContext, identifier); operationError == nil {
					test.Fatal("deleted session remained readable")
				}
				messages, operationError := database.Sessions().Messages(operationContext, identifier)
				testutil.RequireNoError(test, operationError)
				if len(messages) != 0 {
					test.Fatal("conversation content remained")
				}
			}
			remaining, operationError := database.Sessions().List(operationContext, prefix)
			testutil.RequireNoError(test, operationError)
			if len(remaining) != 0 {
				test.Fatal("deleted sessions remain listed")
			}
			_, operationError = database.Sessions().Get(operationContext, foreign.ID)
			testutil.RequireNoError(test, operationError)
			events, operationError := database.Events().Since(operationContext, prefix, 0)
			testutil.RequireNoError(test, operationError)
			if len(events) != 0 {
				test.Fatal("stored event content remained")
			}
			if _, operationError := database.Processes().Get(operationContext, process.ID); operationError == nil {
				test.Fatal("process record remained")
			}
			if _, operationError := database.Permissions().Get(operationContext, decision.RequestID); operationError == nil {
				test.Fatal("session permission remained")
			}
			retained, operationError := database.Permissions().Get(operationContext, persistentDecision.RequestID)
			testutil.RequireNoError(test, operationError)
			if retained.Scope != atom.ScopeAlways || retained.SessionID != "" {
				test.Fatal("workspace approval changed or retained the session reference")
			}
			for label, write := range map[string]func() error{
				"lifecycle save":  func() error { return database.Sessions().Save(operationContext, child) },
				"message append":  func() error { return database.Sessions().Append(operationContext, message) },
				"queue submit":    func() error { return database.Queue().Enqueue(operationContext, message, 128) },
				"event record":    func() error { return database.Events().Append(operationContext, event) },
				"process save":    func() error { return database.Processes().Save(operationContext, process) },
				"permission save": func() error { return database.Permissions().Save(operationContext, decision) },
				"new child": func() error {
					return database.Sessions().Save(operationContext, atom.Session{ID: root.ID + "-late", Parent: root.ID, InstanceID: prefix, CreatedAt: time.Now()})
				},
			} {
				if operationError := write(); !errors.Is(operationError, store.ErrSessionDeleted) {
					test.Fatalf("%s resurrected deleted state: %v", label, operationError)
				}
			}
			for _, statistics := range []func(context.Context) (atom.Statistics, error){
				func(operationContext context.Context) (atom.Statistics, error) {
					return database.Usage().Instance(operationContext, prefix)
				},
				func(operationContext context.Context) (atom.Statistics, error) {
					return database.Usage().Session(operationContext, root.ID)
				},
			} {
				value, operationError := statistics(operationContext)
				testutil.RequireNoError(test, operationError)
				if value.Calls != 1 || value.Input != 17 || value.Cost == nil || value.Cost.Value != 0.25 || !value.Cost.Estimated {
					test.Fatalf("billing was lost: %+v", value)
				}
			}
			busySession := atom.Session{ID: root.ID + "-busy", InstanceID: prefix, CreatedAt: time.Now()}
			testutil.RequireNoError(test, database.Sessions().Save(operationContext, busySession))
			message.ID, message.SessionID = prefix+"-waiting", busySession.ID
			testutil.RequireNoError(test, database.Queue().Enqueue(operationContext, message, 128))
			if _, operationError := database.Sessions().DeleteConversation(operationContext, busySession.ID); !errors.Is(operationError, store.ErrConversationBusy) {
				test.Fatalf("queued work was deleted: %v", operationError)
			}
			_, operationError = database.Queue().ClearPending(operationContext, busySession.ID)
			testutil.RequireNoError(test, operationError)
			process.ID, process.SessionID, process.Status = prefix+"-running", busySession.ID, "running"
			testutil.RequireNoError(test, database.Processes().Save(operationContext, process))
			if _, operationError := database.Sessions().DeleteConversation(operationContext, busySession.ID); !errors.Is(operationError, store.ErrConversationBusy) {
				test.Fatalf("running process was deleted: %v", operationError)
			}
			_, operationError = database.Sessions().Get(operationContext, busySession.ID)
			testutil.RequireNoError(test, operationError)
		})
	}
}
