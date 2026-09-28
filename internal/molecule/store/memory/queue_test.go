package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestQueueBoundAndPendingCancellation(test *testing.T) {
	database := New()
	operationContext := context.Background()
	first := atom.Message{ID: "first", SessionID: "session", Role: atom.RoleUser}
	second := atom.Message{ID: "second", SessionID: "session", Role: atom.RoleUser}
	testutil.RequireNoError(test, database.Queue().Enqueue(operationContext, first, 1))
	if operationError := database.Queue().Enqueue(operationContext, second, 1); !errors.Is(operationError, store.ErrQueueFull) {
		test.Fatalf("queue is unbounded: %v", operationError)
	}
	testutil.RequireNoError(test, database.Queue().Start(operationContext, first.ID))
	removed, operationError := database.Queue().Remove(operationContext, first.SessionID, first.ID)
	testutil.RequireNoError(test, operationError)
	if removed {
		test.Fatal("cancel-queued removed the active turn")
	}
	testutil.RequireNoError(test, database.Queue().Enqueue(operationContext, second, 1))
	removed, operationError = database.Queue().Remove(operationContext, second.SessionID, second.ID)
	testutil.RequireNoError(test, operationError)
	if !removed {
		test.Fatal("pending message not removed")
	}
	messages, operationError := database.Sessions().Messages(operationContext, first.SessionID)
	testutil.RequireNoError(test, operationError)
	if len(messages) != 1 || messages[0].ID != first.ID {
		test.Fatalf("cancelled queue entry entered history: %+v", messages)
	}
}
