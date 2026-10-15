package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestUnlinkMarkerRejectsASessionSaveWaitingOnTheWorkspaceRow(test *testing.T) {
	database := regressionStore(test)
	operationContext := context.Background()
	identifier := fmt.Sprintf("unlink-session-race-%d", time.Now().UnixNano())
	instance := atom.InstanceSpec{ID: identifier, Workspace: test.TempDir(), CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Instances().Save(operationContext, instance))
	transaction, operationError := database.pool.Begin(operationContext)
	testutil.RequireNoError(test, operationError)
	defer transaction.Rollback(operationContext)
	_, operationError = transaction.Exec(operationContext, `SELECT id FROM instances WHERE id=$1 FOR UPDATE`, identifier)
	testutil.RequireNoError(test, operationError)
	entered, completed := make(chan struct{}), make(chan error, 1)
	session := atom.Session{ID: atom.SessionID(identifier + "-late"), InstanceID: identifier, CreatedAt: time.Now()}
	go func() { close(entered); completed <- database.Sessions().Save(operationContext, session) }()
	<-entered
	select {
	case operationError := <-completed:
		test.Fatalf("session save bypassed workspace row gate: %v", operationError)
	case <-time.After(30 * time.Millisecond):
	}
	_, operationError = transaction.Exec(operationContext, `UPDATE instances SET stopped=true,unlinked=true WHERE id=$1`, identifier)
	testutil.RequireNoError(test, operationError)
	testutil.RequireNoError(test, transaction.Commit(operationContext))
	select {
	case operationError := <-completed:
		if !errors.Is(operationError, store.ErrInstanceUnlinked) {
			test.Fatalf("waiting session save crossed unlink: %v", operationError)
		}
	case <-time.After(3 * time.Second):
		test.Fatal("session save did not finish")
	}
	if _, operationError := database.Sessions().Get(operationContext, session.ID); !errors.Is(operationError, store.ErrSessionNotFound) {
		test.Fatal("late session was persisted")
	}
}
