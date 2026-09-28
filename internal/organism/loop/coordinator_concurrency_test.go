package loop_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/permission"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

type controlledQueueStore struct {
	store.QueueStore
	enqueue func(context.Context, atom.Message, int) error
}

func (database controlledQueueStore) Enqueue(operationContext context.Context, message atom.Message, limit int) error {
	return database.enqueue(operationContext, message, limit)
}

type controlledStore struct {
	store.Store
	queue store.QueueStore
}

func (database controlledStore) Queue() store.QueueStore {
	return database.queue
}

func queueWithStorage(test *testing.T, testStack *stack, queueStore store.QueueStore) *loop.Queue {
	test.Helper()
	return queueWithStore(test, testStack, controlledStore{Store: testStack.database, queue: queueStore})
}

func queueWithStore(test *testing.T, testStack *stack, database store.Store) *loop.Queue {
	test.Helper()
	agentLoop := loop.New(testStack.harnessRuntime, loop.Config{
		Gateway: gateway.New(testStack.harnessRuntime), Registry: testStack.registry,
		Store: database,
		Bus:   testStack.bus, Broker: testStack.broker, Engine: permission.NewEngine(), Instances: testStack.instances,
	})
	return newTestQueue(test, agentLoop)
}

type revertingSessions struct {
	store.SessionStore
	entered chan struct{}
}

func (sessions revertingSessions) DeleteAfter(operationContext context.Context, sessionID atom.SessionID, messageID string) (int, error) {
	close(sessions.entered)
	<-operationContext.Done()
	return 0, operationContext.Err()
}

type revertingStore struct {
	store.Store
	sessions store.SessionStore
}

func (database revertingStore) Sessions() store.SessionStore {
	return database.sessions
}

func TestStopOvertakesRevertWithoutReopeningAdmission(test *testing.T) {
	testStack := newStack(test, provider.Text("done"))
	session := testStack.instance(test, 2)
	testStack.user(test, session, "anchor")
	entered := make(chan struct{})
	queue := queueWithStore(test, testStack, revertingStore{Store: testStack.database, sessions: revertingSessions{SessionStore: testStack.database.Sessions(), entered: entered}})
	reverted := make(chan error, 1)
	go func() {
		_, operationError := queue.Revert(context.Background(), session, "user-1")
		reverted <- operationError
	}()
	awaitSignal(test, entered)
	operationContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, operationError := queue.Status(operationContext, session.ID); operationError != nil {
		test.Fatal(operationError)
	}
	testutil.RequireNoError(test, queue.StopInstance(operationContext, session.InstanceID))
	if operationError := <-reverted; !errors.Is(operationError, context.Canceled) {
		test.Fatalf("revert was not interrupted by stop: %v", operationError)
	}
	if _, _, operationError := queue.Submit(operationContext, session, "after stop"); !errors.Is(operationError, loop.ErrInstanceStopped) {
		test.Fatalf("revert reopened a stopped session: %v", operationError)
	}
	testutil.RequireNoError(test, queue.ResumeInstance(operationContext, session.InstanceID))
	_, _, operationError := queue.Submit(operationContext, session, "resumed")
	testutil.RequireNoError(test, operationError)
}

func awaitSignal(test *testing.T, signal <-chan struct{}) {
	test.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		test.Fatal("operation did not reach the expected state")
	}
}

func TestCoordinatorRemainsResponsiveDuringStorageWait(test *testing.T) {
	testStack := newStack(test, provider.Call("wait", `{}`), provider.Text("done"))
	session := testStack.instance(test, 2)
	toolStarted, toolCancelled := make(chan struct{}), make(chan struct{})
	testutil.RequireNoError(test, testStack.registry.Add(&fakeTool{name: "wait", run: func(operationContext context.Context, _ atom.ToolCall) (atom.ToolResult, error) {
		close(toolStarted)
		<-operationContext.Done()
		close(toolCancelled)
		return atom.ToolResult{}, operationContext.Err()
	}}))
	storageStarted, releaseStorage := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() {
		close(releaseStorage)
	})
	defer release()
	queue := queueWithStorage(test, testStack, controlledQueueStore{QueueStore: testStack.database.Queue(), enqueue: func(operationContext context.Context, message atom.Message, limit int) error {
		if message.Content[0].Text == "slow storage" {
			close(storageStarted)
			select {
			case <-releaseStorage:
			case <-operationContext.Done():
				return operationContext.Err()
			}
		}
		return testStack.database.Queue().Enqueue(operationContext, message, limit)
	}})
	_, _, operationError := queue.Submit(context.Background(), session, "start")
	testutil.RequireNoError(test, operationError)
	awaitSignal(test, toolStarted)
	submitted := make(chan error, 1)
	go func() {
		_, _, operationError := queue.Submit(context.Background(), session, "slow storage")
		submitted <- operationError
	}()
	awaitSignal(test, storageStarted)
	operationContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	status, operationError := queue.Status(operationContext, session.ID)
	testutil.RequireNoError(test, operationError)
	if !status.Running {
		test.Fatal("active turn disappeared during enqueue")
	}
	cancelled, operationError := queue.Cancel(operationContext, session.ID)
	testutil.RequireNoError(test, operationError)
	if !cancelled {
		test.Fatal("storage prevented cancellation")
	}
	awaitSignal(test, toolCancelled)
	other := atom.Session{ID: "other-session", InstanceID: session.InstanceID, Model: session.Model, CreatedAt: time.Now()}
	testutil.RequireNoError(test, testStack.database.Sessions().Save(operationContext, other))
	_, _, operationError = queue.Submit(operationContext, other, "independent session")
	testutil.RequireNoError(test, operationError)
	release()
	testutil.RequireNoError(test, <-submitted)
}

func TestAbandonedReplyDoesNotBlockCoordinator(test *testing.T) {
	testStack := newStack(test, provider.Text("done"))
	session := testStack.instance(test, 2)
	entered := make(chan struct{})
	queue := queueWithStorage(test, testStack, controlledQueueStore{QueueStore: testStack.database.Queue(), enqueue: func(operationContext context.Context, message atom.Message, limit int) error {
		if message.Content[0].Text == "abandoned" {
			close(entered)
			<-operationContext.Done()
			return operationContext.Err()
		}
		return testStack.database.Queue().Enqueue(operationContext, message, limit)
	}})
	requestContext, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, _, operationError := queue.Submit(requestContext, session, "abandoned")
		result <- operationError
	}()
	awaitSignal(test, entered)
	cancel()
	if operationError := <-result; !errors.Is(operationError, context.Canceled) {
		test.Fatalf("abandoned request: %v", operationError)
	}
	operationContext, finish := context.WithTimeout(context.Background(), time.Second)
	defer finish()
	_, _, operationError := queue.Submit(operationContext, session, "next request")
	testutil.RequireNoError(test, operationError)
	testutil.RequireNoError(test, queue.Close(operationContext))
}

func TestCloseJoinsLateStorageCompletionAndKeepsPendingMessage(test *testing.T) {
	testStack := newStack(test, provider.Text("must not execute"))
	session := testStack.instance(test, 2)
	entered, releaseStorage := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() {
		close(releaseStorage)
	})
	defer release()
	queue := queueWithStorage(test, testStack, controlledQueueStore{QueueStore: testStack.database.Queue(), enqueue: func(_ context.Context, message atom.Message, limit int) error {
		close(entered)
		<-releaseStorage
		// Model a commit completing after cancellation reached the client.
		return testStack.database.Queue().Enqueue(context.Background(), message, limit)
	}})
	result := make(chan error, 1)
	go func() {
		_, _, operationError := queue.Submit(context.Background(), session, "durable")
		result <- operationError
	}()
	awaitSignal(test, entered)
	closingContext, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if operationError := queue.Close(closingContext); !errors.Is(operationError, context.DeadlineExceeded) {
		test.Fatalf("close did not join storage: %v", operationError)
	}
	if _, _, operationError := queue.Submit(context.Background(), session, "late"); !errors.Is(operationError, loop.ErrQueueClosed) {
		test.Fatalf("closed queue admitted a request: %v", operationError)
	}
	release()
	testutil.RequireNoError(test, <-result)
	operationContext, finish := context.WithTimeout(context.Background(), time.Second)
	defer finish()
	testutil.RequireNoError(test, queue.Close(operationContext))
	entries, operationError := testStack.database.Queue().All(operationContext)
	testutil.RequireNoError(test, operationError)
	if len(entries) != 1 || entries[0].Running || testStack.provider.Calls() != 0 {
		test.Fatalf("shutdown lost or ran a pending request: entries=%+v calls=%d", entries, testStack.provider.Calls())
	}
}

func TestWorkerCallbacksCanQueryTheirSession(test *testing.T) {
	testStack := newStack(test, provider.Text("done"))
	session := testStack.instance(test, 2)
	queue := newTestQueue(test, testStack.loop)
	observed := make(chan error, 1)
	testStack.bus.On(atom.EventTurnStart, func(operationContext context.Context, _ atom.Event) {
		operationContext, cancel := context.WithTimeout(operationContext, time.Second)
		defer cancel()
		_, operationError := queue.Status(operationContext, session.ID)
		observed <- operationError
	})
	_, _, operationError := queue.Submit(context.Background(), session, "start")
	testutil.RequireNoError(test, operationError)
	select {
	case operationError := <-observed:
		testutil.RequireNoError(test, operationError)
	case <-time.After(3 * time.Second):
		test.Fatal("callback and coordinator waited on each other")
	}
}

func TestAcceptedStopContinuesAfterCallerDeadline(test *testing.T) {
	testStack := newStack(test, provider.Text("pending"))
	session := testStack.instance(test, 2)
	entered, releaseStorage := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() {
		close(releaseStorage)
	})
	defer release()
	queue := queueWithStorage(test, testStack, controlledQueueStore{QueueStore: testStack.database.Queue(), enqueue: func(operationContext context.Context, message atom.Message, limit int) error {
		close(entered)
		<-releaseStorage
		return testStack.database.Queue().Enqueue(operationContext, message, limit)
	}})
	submitted := make(chan error, 1)
	go func() {
		_, _, operationError := queue.Submit(context.Background(), session, "pending")
		submitted <- operationError
	}()
	awaitSignal(test, entered)
	stopContext, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if operationError := queue.StopInstance(stopContext, session.InstanceID); !errors.Is(operationError, context.DeadlineExceeded) {
		test.Fatalf("stop did not wait for accepted persistence: %v", operationError)
	}
	release()
	testutil.RequireNoError(test, <-submitted)
	operationContext, finish := context.WithTimeout(context.Background(), time.Second)
	defer finish()
	// The original stop continues after its caller leaves. A subsequent caller
	// can join once the directory has processed its completion.
	for {
		operationError := queue.StopInstance(operationContext, session.InstanceID)
		if errors.Is(operationError, loop.ErrSessionBusy) {
			select {
			case <-operationContext.Done():
				test.Fatal("stop did not finish")
			case <-time.After(time.Millisecond):
			}
			continue
		}
		testutil.RequireNoError(test, operationError)
		break
	}
	if _, _, operationError := queue.Submit(operationContext, session, "late"); !errors.Is(operationError, loop.ErrInstanceStopped) {
		test.Fatalf("caller timeout reopened admission: %v", operationError)
	}
	if testStack.provider.Calls() != 0 {
		test.Fatal("pending message ran while the instance was stopped")
	}
}
