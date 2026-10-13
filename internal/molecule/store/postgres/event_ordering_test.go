package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestIndependentSessionCommitsCannotSkipReplay(test *testing.T) {
	database := regressionStore(test)
	operationContext := context.Background()
	workspace := fmt.Sprintf("event-order-%d", time.Now().UnixNano())
	slowID, otherID := atom.SessionID(workspace+"-slow"), atom.SessionID(workspace+"-other")
	// regressionStore deliberately has a one-connection pool. The blocked
	// transaction needs its own connection to test ordering rather than pool size.
	blocker, operationError := pgx.ConnectConfig(operationContext, database.pool.Config().ConnConfig.Copy())
	testutil.RequireNoError(test, operationError)
	defer blocker.Close(operationContext)
	transaction, operationError := blocker.Begin(operationContext)
	testutil.RequireNoError(test, operationError)
	defer transaction.Rollback(operationContext)
	// Allocate a lower sequence that remains uncommitted while another session
	// commits and replays a later sequence in the same workspace.
	_, operationError = transaction.Exec(operationContext, `SELECT pg_advisory_xact_lock(hashtextextended('mtt:event-session:' || $1,0))`, string(slowID))
	testutil.RequireNoError(test, operationError)
	var lowerSequence uint64
	operationError = transaction.QueryRow(operationContext, `INSERT INTO events(instance_id,session_id,name,created_at) VALUES($1,$2,'test',now()) RETURNING seq`, workspace, string(slowID)).Scan(&lowerSequence)
	testutil.RequireNoError(test, operationError)
	later, operationError := database.Events().Record(operationContext, atom.Event{InstanceID: workspace, SessionID: otherID, Name: "test", Time: time.Now()})
	testutil.RequireNoError(test, operationError)
	if later.Seq <= lowerSequence {
		test.Fatal("fixture did not create out-of-order commits")
	}
	events, operationError := database.Events().SinceSession(operationContext, slowID, 0)
	testutil.RequireNoError(test, operationError)
	if len(events) != 0 {
		test.Fatal("session replay included a different session's event")
	}
	blocked := make(chan error, 1)
	go func() {
		_, operationError := database.Events().Record(operationContext, atom.Event{InstanceID: workspace, SessionID: slowID, Name: "test", Time: time.Now()})
		blocked <- operationError
	}()
	select {
	case operationError := <-blocked:
		test.Fatalf("same-session record passed uncommitted predecessor: %v", operationError)
	case <-time.After(25 * time.Millisecond):
	}
	testutil.RequireNoError(test, transaction.Commit(operationContext))
	select {
	case operationError := <-blocked:
		testutil.RequireNoError(test, operationError)
	case <-time.After(time.Second):
		test.Fatal("session record did not resume")
	}
	events, operationError = database.Events().SinceSession(operationContext, slowID, 0)
	testutil.RequireNoError(test, operationError)
	if len(events) != 2 || events[0].Seq != lowerSequence || events[1].Seq <= events[0].Seq {
		test.Fatalf("replay lost ordering: %+v", events)
	}
}
