package loop_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func saveLinkedWorkspace(test *testing.T, testStack *stack, session atom.Session) atom.InstanceSpec {
	test.Helper()
	instance, _ := testStack.instances.Get(session.InstanceID)
	testutil.RequireNoError(test, testStack.database.Instances().Save(context.Background(), instance.Spec()))
	return instance.Spec()
}

func TestUnlinkWorkspacePreservesConversationFilesAndRejectsFurtherWork(test *testing.T) {
	testStack := newStack(test, provider.Text("done"))
	session := testStack.instance(test, 2)
	instance := saveLinkedWorkspace(test, testStack, session)
	testStack.user(test, session, "keep conversation")
	path := filepath.Join(instance.Workspace, "untouched.txt")
	testutil.RequireNoError(test, os.WriteFile(path, []byte("do not delete\n"), 0600))
	link := filepath.Join(instance.Workspace, "keep-link")
	testutil.RequireNoError(test, os.Symlink(path, link))
	before, operationError := testStack.database.Sessions().Messages(context.Background(), session.ID)
	testutil.RequireNoError(test, operationError)
	queue := newTestQueue(test, testStack.loop)
	testutil.RequireNoError(test, queue.UnlinkInstance(context.Background(), instance.ID))
	if testStack.instances.IsRunning(instance.ID) {
		test.Fatal("unlinked workspace remains active")
	}
	after, operationError := testStack.database.Sessions().Messages(context.Background(), session.ID)
	testutil.RequireNoError(test, operationError)
	if !reflect.DeepEqual(before, after) {
		test.Fatal("unlink changed saved conversation")
	}
	content, operationError := os.ReadFile(path)
	testutil.RequireNoError(test, operationError)
	if string(content) != "do not delete\n" {
		test.Fatal("unlink changed filesystem content")
	}
	linkTarget, operationError := os.Readlink(link)
	testutil.RequireNoError(test, operationError)
	if linkTarget != path {
		test.Fatal("unlink changed a symbolic link")
	}
	if _, _, operationError = queue.Submit(context.Background(), session, "late message"); !errors.Is(operationError, store.ErrInstanceUnlinked) {
		test.Fatalf("late submission: %v", operationError)
	}
	if operationError = testStack.loop.Run(context.Background(), session); !errors.Is(operationError, store.ErrInstanceUnlinked) {
		test.Fatalf("late direct run: %v", operationError)
	}
	if _, operationError = queue.ResumeStoredInstance(context.Background(), instance.ID); !errors.Is(operationError, store.ErrInstanceUnlinked) {
		test.Fatalf("unlinked workspace resumed: %v", operationError)
	}
	// Reopening the folder creates a new reference, not a stale revival of the old ID.
	instance.ID = ""
	linked, operationError := testStack.instances.Start(context.Background(), instance)
	testutil.RequireNoError(test, operationError)
	if linked.ID() == session.InstanceID {
		test.Fatal("old workspace ID was reused")
	}
}

func TestUnlinkRejectsActiveAndPendingTurnsWithoutCancellingThem(test *testing.T) {
	testStack := newStack(test, provider.Call("wait", `{}`), provider.Text("done"))
	session := testStack.instance(test, 2)
	saveLinkedWorkspace(test, testStack, session)
	started, cancelled := make(chan struct{}), make(chan struct{})
	testutil.RequireNoError(test, testStack.registry.Add(&fakeTool{name: "wait", run: func(operationContext context.Context, _ atom.ToolCall) (atom.ToolResult, error) {
		close(started)
		<-operationContext.Done()
		close(cancelled)
		return atom.ToolResult{}, operationContext.Err()
	}}))
	queue := newTestQueue(test, testStack.loop)
	_, _, operationError := queue.Submit(context.Background(), session, "start")
	testutil.RequireNoError(test, operationError)
	awaitSignal(test, started)
	_, _, operationError = queue.Submit(context.Background(), session, "pending")
	testutil.RequireNoError(test, operationError)
	if operationError = queue.UnlinkInstance(context.Background(), session.InstanceID); !errors.Is(operationError, store.ErrInstanceBusy) {
		test.Fatalf("active workspace unlinked: %v", operationError)
	}
	select {
	case <-cancelled:
		test.Fatal("unlink cancelled the active turn")
	default:
	}
	if !testStack.instances.IsRunning(session.InstanceID) {
		test.Fatal("busy workspace was stopped")
	}
	status, operationError := queue.Status(context.Background(), session.ID)
	testutil.RequireNoError(test, operationError)
	if len(status.Messages) != 1 || !status.Running {
		test.Fatalf("busy queue changed: %#v", status)
	}
}

func TestUnlinkFencesDirectChildRuns(test *testing.T) {
	testStack := newStack(test, provider.Call("wait", `{}`))
	session := testStack.instance(test, 2)
	saveLinkedWorkspace(test, testStack, session)
	child := session
	child.ID = "unlink-child"
	child.Parent = session.ID
	testutil.RequireNoError(test, testStack.database.Sessions().Save(context.Background(), child))
	started := make(chan struct{})
	testutil.RequireNoError(test, testStack.registry.Add(&fakeTool{name: "wait", run: func(operationContext context.Context, _ atom.ToolCall) (atom.ToolResult, error) {
		close(started)
		<-operationContext.Done()
		return atom.ToolResult{}, operationContext.Err()
	}}))
	operationContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- testStack.loop.Run(operationContext, child) }()
	awaitSignal(test, started)
	queue := newTestQueue(test, testStack.loop)
	if operationError := queue.UnlinkInstance(context.Background(), session.InstanceID); !errors.Is(operationError, store.ErrInstanceBusy) {
		test.Fatalf("active direct child was not fenced: %v", operationError)
	}
	cancel()
	<-done
}

type blockedInstanceStore struct {
	store.InstanceStore
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (database *blockedInstanceStore) Get(operationContext context.Context, identifier string) (atom.InstanceSpec, error) {
	database.once.Do(func() { close(database.entered); <-database.release })
	return database.InstanceStore.Get(operationContext, identifier)
}

type instanceControlledStore struct {
	store.Store
	instances store.InstanceStore
}

func (database instanceControlledStore) Instances() store.InstanceStore { return database.instances }

func TestAcceptedUnlinkOutlivesCallerAndJoinsQueueShutdown(test *testing.T) {
	testStack := newStack(test, provider.Text("done"))
	session := testStack.instance(test, 2)
	saveLinkedWorkspace(test, testStack, session)
	blocked := &blockedInstanceStore{InstanceStore: testStack.database.Instances(), entered: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(blocked.release) })
	defer release()
	queue := queueWithStore(test, testStack, instanceControlledStore{Store: testStack.database, instances: blocked})
	operationContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- queue.UnlinkInstance(operationContext, session.InstanceID) }()
	awaitSignal(test, blocked.entered)
	cancel()
	if operationError := <-result; !errors.Is(operationError, context.Canceled) {
		test.Fatalf("caller did not abandon its wait: %v", operationError)
	}
	if _, operationError := queue.ResumeStoredInstance(context.Background(), session.InstanceID); !errors.Is(operationError, loop.ErrSessionBusy) {
		test.Fatalf("resume bypassed unlink gate: %v", operationError)
	}
	probeContext, stopProbe := context.WithTimeout(context.Background(), time.Second)
	defer stopProbe()
	if _, operationError := queue.Status(probeContext, "unrelated"); operationError != nil {
		test.Fatalf("directory was blocked by storage: %v", operationError)
	}
	closed := make(chan error, 1)
	go func() { closed <- queue.Close(context.Background()) }()
	select {
	case <-closed:
		test.Fatal("shutdown did not join accepted unlink")
	case <-time.After(20 * time.Millisecond):
	}
	release()
	testutil.RequireNoError(test, <-closed)
	if _, operationError := testStack.database.Instances().Get(context.Background(), session.InstanceID); !errors.Is(operationError, store.ErrInstanceNotFound) {
		test.Fatalf("accepted unlink was abandoned: %v", operationError)
	}
}

func TestStoredStopAndResumePreserveTheWorkspaceReference(test *testing.T) {
	testStack := newStack(test, provider.Text("done"))
	session := testStack.instance(test, 2)
	saveLinkedWorkspace(test, testStack, session)
	queue := newTestQueue(test, testStack.loop)
	testutil.RequireNoError(test, queue.StopStoredInstance(context.Background(), session.InstanceID))
	instance, operationError := testStack.database.Instances().Get(context.Background(), session.InstanceID)
	testutil.RequireNoError(test, operationError)
	if !instance.Stopped {
		test.Fatal("stop was not persisted")
	}
	resumed, operationError := queue.ResumeStoredInstance(context.Background(), session.InstanceID)
	testutil.RequireNoError(test, operationError)
	if resumed.Stopped || !testStack.instances.IsRunning(session.InstanceID) {
		test.Fatal("workspace did not resume")
	}
}

type rejectedUnlinkStore struct{ store.InstanceStore }

func (database rejectedUnlinkStore) Unlink(context.Context, string) error {
	return errors.New("unlink storage failure")
}

func TestUnlinkStorageFailureReleasesAdmission(test *testing.T) {
	testStack := newStack(test, provider.Text("after failed unlink"))
	session := testStack.instance(test, 2)
	saveLinkedWorkspace(test, testStack, session)
	testStack.user(test, session, "anchor")
	queue := queueWithStore(test, testStack, instanceControlledStore{Store: testStack.database, instances: rejectedUnlinkStore{testStack.database.Instances()}})
	_, operationError := queue.Revert(context.Background(), session, "user-1")
	testutil.RequireNoError(test, operationError)
	if operationError = queue.UnlinkInstance(context.Background(), session.InstanceID); operationError == nil {
		test.Fatal("storage failure was hidden")
	}
	if !testStack.instances.IsRunning(session.InstanceID) {
		test.Fatal("failed unlink stopped workspace")
	}
	_, _, operationError = queue.Submit(context.Background(), session, "accepted after failure")
	testutil.RequireNoError(test, operationError)
}

func TestUnlinkStoppedWorkspaceDoesNotRequireItsDirectory(test *testing.T) {
	testStack := newStack(test)
	instance := atom.InstanceSpec{ID: "unavailable-workspace", Workspace: filepath.Join(test.TempDir(), "missing-directory"), Stopped: true, CreatedAt: time.Now()}
	testutil.RequireNoError(test, testStack.database.Instances().Save(context.Background(), instance))
	queue := newTestQueue(test, testStack.loop)
	testutil.RequireNoError(test, queue.UnlinkInstance(context.Background(), instance.ID))
	if _, operationError := os.Stat(instance.Workspace); !errors.Is(operationError, os.ErrNotExist) {
		test.Fatal("unlink created or accessed a new workspace folder")
	}
}

type rejectedInstanceSave struct{ store.InstanceStore }

func (database rejectedInstanceSave) Save(context.Context, atom.InstanceSpec) error {
	return errors.New("instance persistence failure")
}

func TestFailedStoredStopDoesNotCloseNewSessionAdmission(test *testing.T) {
	testStack := newStack(test, provider.Text("still active"))
	session := testStack.instance(test, 2)
	saveLinkedWorkspace(test, testStack, session)
	queue := queueWithStore(test, testStack, instanceControlledStore{Store: testStack.database, instances: rejectedInstanceSave{testStack.database.Instances()}})
	if operationError := queue.StopStoredInstance(context.Background(), session.InstanceID); operationError == nil {
		test.Fatal("failed persistence was hidden")
	}
	if !testStack.instances.IsRunning(session.InstanceID) {
		test.Fatal("failed stop removed active workspace")
	}
	_, _, operationError := queue.Submit(context.Background(), session, "still accepted")
	testutil.RequireNoError(test, operationError)
}
