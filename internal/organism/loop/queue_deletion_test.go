package loop_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/eventbus"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/registry"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestQueueDeletionRejectsActiveTurnsAndFencesDeletedIDs(test *testing.T) {
	testStack := newStack(test, provider.Text("slow"))
	testStack.provider.SetDelay(time.Second)
	session := testStack.instance(test, 2)
	queue := newTestQueue(test, testStack.loop)
	operationContext := context.Background()
	_, _, operationError := queue.Submit(operationContext, session, "active")
	testutil.RequireNoError(test, operationError)
	if _, operationError := queue.DeleteConversation(operationContext, session); !errors.Is(operationError, store.ErrConversationBusy) {
		test.Fatalf("active turn was not protected: %v", operationError)
	}
	_, operationError = queue.Cancel(operationContext, session.ID)
	testutil.RequireNoError(test, operationError)
	deadline := time.Now().Add(3 * time.Second)
	for {
		status, operationError := queue.Status(operationContext, session.ID)
		testutil.RequireNoError(test, operationError)
		if !status.Running && len(status.Messages) == 0 {
			break
		}
		if time.Now().After(deadline) {
			test.Fatal("cancelled session did not stop")
		}
		time.Sleep(time.Millisecond)
	}
	deleted, operationError := queue.DeleteConversation(operationContext, session)
	testutil.RequireNoError(test, operationError)
	if len(deleted) != 1 || deleted[0] != session.ID {
		test.Fatalf("deleted: %v", deleted)
	}
	if _, _, operationError := queue.Submit(operationContext, session, "late input"); !errors.Is(operationError, store.ErrSessionDeleted) {
		test.Fatalf("stale submission accepted: %v", operationError)
	}
	if operationError := testStack.loop.Run(operationContext, session); !errors.Is(operationError, store.ErrSessionDeleted) {
		test.Fatalf("deleted turn restarted: %v", operationError)
	}
	if _, operationError := testStack.database.Sessions().Get(operationContext, session.ID); operationError == nil {
		test.Fatal("session returned after deletion")
	}
}

type deletionPauseStore struct {
	store.Store
	started    chan struct{}
	release    chan struct{}
	failure    error
	beforeList bool
	once       sync.Once
}

func (database *deletionPauseStore) Sessions() store.SessionStore {
	return &deletionPauseSessions{SessionStore: database.Store.Sessions(), database: database}
}

type deletionPauseSessions struct {
	store.SessionStore
	database *deletionPauseStore
}

func (sessions *deletionPauseSessions) List(operationContext context.Context, instanceID string) ([]atom.Session, error) {
	if sessions.database.beforeList {
		sessions.database.pause()
	}
	return sessions.SessionStore.List(operationContext, instanceID)
}

func (sessions *deletionPauseSessions) DeleteConversation(operationContext context.Context, identifier atom.SessionID) ([]atom.SessionID, error) {
	if !sessions.database.beforeList {
		sessions.database.pause()
	}
	if sessions.database.failure != nil {
		return nil, sessions.database.failure
	}
	return sessions.SessionStore.DeleteConversation(operationContext, identifier)
}

func (database *deletionPauseStore) pause() {
	database.once.Do(func() { close(database.started); <-database.release })
}

func newDeletionTestLoop(test *testing.T, database store.Store) *loop.Loop {
	test.Helper()
	harnessRuntime := harness.New()
	modelGateway := gateway.New(harnessRuntime)
	testutil.RequireNoError(test, modelGateway.Add(provider.NewTest("test", provider.Text("done"))))
	return loop.New(harnessRuntime, loop.Config{Store: database, Gateway: modelGateway, Registry: registry.New(harnessRuntime, nil), Bus: eventbus.New(harnessRuntime)})
}

func TestDeletionDoesNotBlockUnrelatedSessionsAndSurvivesAbandonedWait(test *testing.T) {
	for _, beforeList := range []bool{true, false} {
		test.Run(map[bool]string{true: "tree-read", false: "delete-transaction"}[beforeList], func(test *testing.T) {
			database := &deletionPauseStore{Store: memory.New(), started: make(chan struct{}), release: make(chan struct{}), beforeList: beforeList}
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(database.release) })
			runner := newDeletionTestLoop(test, database)
			queue := newTestQueue(test, runner)
			operationContext := context.Background()
			root := atom.Session{ID: "old", InstanceID: "workspace", Model: "test-model"}
			other := atom.Session{ID: "other", InstanceID: root.InstanceID, Model: "test-model"}
			testutil.RequireNoError(test, database.Store.Sessions().Save(operationContext, root))
			testutil.RequireNoError(test, database.Store.Sessions().Save(operationContext, other))
			callerContext, cancel := context.WithCancel(operationContext)
			result := make(chan error, 1)
			go func() { _, operationError := queue.DeleteConversation(callerContext, root); result <- operationError }()
			select {
			case <-database.started:
			case <-time.After(time.Second):
				test.Fatal("deletion worker did not start")
			}
			cancel()
			if operationError := <-result; !errors.Is(operationError, context.Canceled) {
				test.Fatalf("caller wait: %v", operationError)
			}
			requestContext, stop := context.WithTimeout(operationContext, time.Second)
			defer stop()
			_, _, operationError := queue.Submit(requestContext, other, "unrelated work")
			testutil.RequireNoError(test, operationError)
			if !beforeList {
				if _, _, operationError := queue.Submit(requestContext, root, "late input"); !errors.Is(operationError, loop.ErrSessionBusy) {
					test.Fatalf("admission fence failed: %v", operationError)
				}
				if operationError := runner.Run(requestContext, root); !errors.Is(operationError, store.ErrConversationBusy) {
					test.Fatalf("direct run fence failed: %v", operationError)
				}
			}
			releaseOnce.Do(func() { close(database.release) })
			testutil.RequireNoError(test, queue.Close(operationContext))
			if _, operationError := database.Store.Sessions().Get(operationContext, root.ID); operationError == nil {
				test.Fatal("accepted deletion was abandoned with its caller")
			}
		})
	}
}

func TestDeletionStorageFailureReopensAdmission(test *testing.T) {
	database := &deletionPauseStore{Store: memory.New(), started: make(chan struct{}), release: make(chan struct{}), failure: errors.New("storage unavailable")}
	close(database.release)
	runner := newDeletionTestLoop(test, database)
	queue := newTestQueue(test, runner)
	operationContext := context.Background()
	session := atom.Session{ID: "preserved", InstanceID: "workspace", Model: "test-model"}
	testutil.RequireNoError(test, database.Store.Sessions().Save(operationContext, session))
	if _, operationError := queue.DeleteConversation(operationContext, session); !errors.Is(operationError, database.failure) {
		test.Fatalf("missing storage error: %v", operationError)
	}
	_, _, operationError := queue.Submit(operationContext, session, "retry work")
	testutil.RequireNoError(test, operationError)
}
