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

func TestQueueBatchClaimsAreAtomicAndOrdered(test *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		test.Run(backend, func(test *testing.T) {
			var database store.Store = memory.New()
			if backend == "postgres" {
				url := os.Getenv("MTT_TEST_DATABASE_URL")
				if url == "" {
					test.Skip("PostgreSQL test database is not configured")
				}
				opened, operationError := postgres.Open(context.Background(), url)
				testutil.RequireNoError(test, operationError)
				database = opened
				test.Cleanup(func() { opened.Close() })
			}
			operationContext := context.Background()
			session := atom.Session{ID: atom.SessionID(fmt.Sprintf("batch-%d", time.Now().UnixNano())), InstanceID: "batch-workspace", CreatedAt: time.Now()}
			testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
			test.Cleanup(func() {
				_, operationError := database.Sessions().DeleteConversation(context.Background(), session.ID)
				testutil.RequireNoError(test, operationError)
			})
			identifiers := []string{string(session.ID) + "-one", string(session.ID) + "-two", string(session.ID) + "-runtime"}
			for index, identifier := range identifiers {
				role := atom.RoleUser
				if index == 2 {
					role = atom.RoleRuntime
				}
				testutil.RequireNoError(test, database.Queue().Enqueue(operationContext, atom.Message{ID: identifier, SessionID: session.ID, Role: role, Content: []atom.Content{{Type: atom.Text, Text: identifier}}, CreatedAt: time.Now()}, 128))
			}
			for _, invalid := range [][]string{{identifiers[0], "missing"}, {identifiers[1], identifiers[0]}, {identifiers[0], identifiers[0]}} {
				if database.Queue().StartBatch(operationContext, session.ID, invalid) == nil {
					test.Fatal("invalid batch succeeded")
				}
				messages, operationError := database.Sessions().Messages(operationContext, session.ID)
				testutil.RequireNoError(test, operationError)
				if len(messages) != 0 {
					test.Fatal("failed batch appended partial history")
				}
				entries, operationError := database.Queue().All(operationContext)
				testutil.RequireNoError(test, operationError)
				for _, entry := range entries {
					if entry.Message.SessionID == session.ID && entry.Running {
						test.Fatal("failed batch claimed a message")
					}
				}
			}
			pending, readError := database.Queue().PendingMessage(operationContext, session.ID, identifiers[0])
			testutil.RequireNoError(test, readError)
			if pending.Content[0].Text != identifiers[0] || pending.Role != atom.RoleUser {
				test.Fatal("pending-message read changed input")
			}
			pending.Content[0].Text = "local change"
			again, readError := database.Queue().PendingMessage(operationContext, session.ID, identifiers[0])
			testutil.RequireNoError(test, readError)
			if again.Content[0].Text != identifiers[0] {
				test.Fatal("pending read aliases stored content")
			}
			if _, readError = database.Queue().PendingMessage(operationContext, "foreign-session", identifiers[0]); !errors.Is(readError, store.ErrQueuedMessageNotFound) {
				test.Fatal("pending read crossed sessions")
			}
			testutil.RequireNoError(test, database.Queue().StartBatch(operationContext, session.ID, identifiers))
			if _, readError = database.Queue().PendingMessage(operationContext, session.ID, identifiers[0]); !errors.Is(readError, store.ErrQueuedMessageNotFound) {
				test.Fatal("admitted input remained readable as pending")
			}
			messages, operationError := database.Sessions().Messages(operationContext, session.ID)
			testutil.RequireNoError(test, operationError)
			if len(messages) != 3 {
				test.Fatal("missing batch messages")
			}
			for index, message := range messages {
				if message.ID != identifiers[index] {
					test.Fatal("batch order changed")
				}
			}
			if messages[2].Role != atom.RoleRuntime {
				test.Fatal("runtime role changed")
			}
			if removed, operationError := database.Queue().Remove(operationContext, session.ID, identifiers[1]); operationError != nil || removed {
				test.Fatal("claimed message remained cancellable")
			}
			if database.Queue().StartBatch(operationContext, session.ID, identifiers) == nil {
				test.Fatal("batch was appended twice")
			}
			for _, identifier := range identifiers {
				testutil.RequireNoError(test, database.Queue().Finish(operationContext, identifier))
			}
		})
	}
}
