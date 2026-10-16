package loop_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

type controlledBatchQueue struct {
	store.QueueStore
	startBatch func(context.Context, atom.SessionID, []string) error
}

func (queue controlledBatchQueue) StartBatch(operationContext context.Context, sessionID atom.SessionID, identifiers []string) error {
	return queue.startBatch(operationContext, sessionID, identifiers)
}

func TestCancellationJoinsCommittedBatchAndDoesNotReplayIt(test *testing.T) {
	testStack := newStack(test, provider.Call("gate", `{}`), provider.Text("must not run"))
	session := testStack.instance(test, 2)
	toolStarted, toolRelease := make(chan struct{}), make(chan struct{})
	releaseTool := sync.OnceFunc(func() { close(toolRelease) })
	defer releaseTool()
	testutil.RequireNoError(test, testStack.registry.Add(&fakeTool{name: "gate", run: func(operationContext context.Context, _ atom.ToolCall) (atom.ToolResult, error) {
		close(toolStarted)
		select {
		case <-toolRelease:
			return atom.ToolResult{Status: atom.StatusOK}, nil
		case <-operationContext.Done():
			return atom.ToolResult{}, operationContext.Err()
		}
	}}))
	committed, replyRelease := make(chan struct{}), make(chan struct{})
	releaseReply := sync.OnceFunc(func() { close(replyRelease) })
	defer releaseReply()
	queue := queueWithStorage(test, testStack, controlledBatchQueue{QueueStore: testStack.database.Queue(), startBatch: func(operationContext context.Context, sessionID atom.SessionID, identifiers []string) error {
		operationError := testStack.database.Queue().StartBatch(operationContext, sessionID, identifiers)
		if operationError != nil {
			return operationError
		}
		close(committed)
		<-replyRelease
		// The transaction committed, but cancellation hid its acknowledgement.
		return operationContext.Err()
	}})
	_, _, operationError := queue.Submit(context.Background(), session, "initial")
	testutil.RequireNoError(test, operationError)
	awaitSignal(test, toolStarted)
	for _, text := range []string{"one", "two", "three"} {
		_, _, operationError = queue.Submit(context.Background(), session, text)
		testutil.RequireNoError(test, operationError)
	}
	releaseTool()
	awaitSignal(test, committed)
	operationContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cancelled, operationError := queue.Cancel(operationContext, session.ID)
	testutil.RequireNoError(test, operationError)
	if !cancelled {
		test.Fatal("active run was not cancelled")
	}
	status, operationError := queue.Status(operationContext, session.ID)
	testutil.RequireNoError(test, operationError)
	if !status.Running {
		test.Fatal("run left an in-flight admission commit behind")
	}
	releaseReply()
	awaitQueuedIdle(test, queue, session.ID)
	if testStack.provider.Calls() != 1 {
		test.Fatal("cancelled batch was sent to the model")
	}
	entries, operationError := testStack.database.Queue().All(context.Background())
	testutil.RequireNoError(test, operationError)
	if len(entries) != 0 {
		test.Fatal("committed admission leaked running queue rows")
	}
	messages, operationError := testStack.database.Sessions().Messages(context.Background(), session.ID)
	testutil.RequireNoError(test, operationError)
	users := 0
	for _, message := range messages {
		if message.Role == atom.RoleUser {
			users++
		}
	}
	if users != 4 {
		test.Fatal("committed user input was lost")
	}
}

func TestFailedAdmissionKeepsPendingMessagesAndCanResume(test *testing.T) {
	testStack := newStack(test, provider.Call("gate", `{}`), provider.Text("resumed batch"))
	session := testStack.instance(test, 2)
	started, continued := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(continued) })
	defer release()
	testutil.RequireNoError(test, testStack.registry.Add(&fakeTool{name: "gate", run: func(operationContext context.Context, _ atom.ToolCall) (atom.ToolResult, error) {
		close(started)
		select {
		case <-continued:
			return atom.ToolResult{Status: atom.StatusOK}, nil
		case <-operationContext.Done():
			return atom.ToolResult{}, operationContext.Err()
		}
	}}))
	var failOnce sync.Once
	queue := queueWithStorage(test, testStack, controlledBatchQueue{QueueStore: testStack.database.Queue(), startBatch: func(operationContext context.Context, sessionID atom.SessionID, identifiers []string) error {
		fail := false
		failOnce.Do(func() { fail = true })
		if fail {
			return errors.New("batch storage unavailable")
		}
		return testStack.database.Queue().StartBatch(operationContext, sessionID, identifiers)
	}})
	_, _, operationError := queue.Submit(context.Background(), session, "initial")
	testutil.RequireNoError(test, operationError)
	awaitSignal(test, started)
	for _, text := range []string{"one", "two", "three"} {
		_, _, operationError = queue.Submit(context.Background(), session, text)
		testutil.RequireNoError(test, operationError)
	}
	release()
	deadline := time.Now().Add(3 * time.Second)
	for {
		status, readError := queue.Status(context.Background(), session.ID)
		testutil.RequireNoError(test, readError)
		if !status.Running && status.Error != "" {
			if len(status.Messages) != 3 {
				test.Fatal("failed admission removed pending inputs")
			}
			break
		}
		if time.Now().After(deadline) {
			test.Fatal("failed admission did not stop")
		}
		time.Sleep(time.Millisecond)
	}
	testutil.RequireNoError(test, queue.ResumeInstance(context.Background(), session.InstanceID))
	awaitQueuedIdle(test, queue, session.ID)
	if testStack.provider.Calls() != 2 {
		test.Fatal("resumed batch ran as separate turns")
	}
}

func awaitQueuedIdle(test *testing.T, queue *loop.Queue, sessionID atom.SessionID) {
	test.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		status, operationError := queue.Status(context.Background(), sessionID)
		testutil.RequireNoError(test, operationError)
		if !status.Running && len(status.Messages) == 0 {
			if status.Error != "" {
				test.Fatal(status.Error)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	test.Fatal("queue did not become idle")
}

func TestQueuedMessagesJoinNextModelRequestAfterCompleteToolGroup(test *testing.T) {
	testStack := newStack(test, provider.Call("gate", `{}`), provider.Text("all messages handled"))
	session := testStack.instance(test, 2)
	started, continued := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(continued) })
	defer release()
	testutil.RequireNoError(test, testStack.registry.Add(&fakeTool{name: "gate", run: func(operationContext context.Context, _ atom.ToolCall) (atom.ToolResult, error) {
		close(started)
		select {
		case <-continued:
			return atom.ToolResult{Status: atom.StatusOK, Content: []atom.Content{{Type: atom.Text, Text: "finished tool group"}}}, nil
		case <-operationContext.Done():
			return atom.ToolResult{}, operationContext.Err()
		}
	}}))
	queue := newTestQueue(test, testStack.loop)
	operationContext := context.Background()
	_, _, operationError := queue.Submit(operationContext, session, "initial")
	testutil.RequireNoError(test, operationError)
	awaitSignal(test, started)
	var identifiers []string
	for _, text := range []string{"follow-up one", "follow-up two", "follow-up three"} {
		message, _, submitError := queue.Submit(operationContext, session, text)
		testutil.RequireNoError(test, submitError)
		identifiers = append(identifiers, message.ID)
	}
	notice, _, operationError := queue.SubmitRuntimeContent(operationContext, session, []atom.Content{{Type: atom.Text, Text: "process finished"}})
	testutil.RequireNoError(test, operationError)
	if testStack.provider.Calls() != 1 {
		test.Fatal("queued messages interrupted the active tool group")
	}
	release()
	awaitQueuedIdle(test, queue, session.ID)
	if testStack.provider.Calls() != 2 {
		test.Fatalf("queued inputs caused separate requests: %d", testStack.provider.Calls())
	}
	request := testStack.provider.Requests[1]
	var observed []string
	toolIndex := -1
	firstUser := -1
	for index, message := range request.Messages {
		if message.Role == atom.RoleTool {
			toolIndex = index
		}
		for _, identifier := range identifiers {
			if message.ID == identifier {
				if firstUser < 0 {
					firstUser = index
				}
				observed = append(observed, message.ID)
			}
		}
		if message.ID == notice.ID && message.Role != atom.RoleRuntime {
			test.Fatal("runtime data became user input")
		}
	}
	if len(observed) != 3 || toolIndex < 0 || firstUser <= toolIndex {
		test.Fatalf("batch missing or ahead of tool results: %v", observed)
	}
	for index, identifier := range identifiers {
		if observed[index] != identifier {
			test.Fatal("queued message order changed")
		}
	}
	entries, operationError := testStack.database.Queue().All(operationContext)
	testutil.RequireNoError(test, operationError)
	if len(entries) != 0 {
		test.Fatal("claimed messages remained in the durable queue")
	}
}

func TestQueuedMessagesAfterTextResponseUseOneNextRequest(test *testing.T) {
	testStack := newStack(test, provider.Text("first response"), provider.Text("one combined follow-up"))
	session := testStack.instance(test, 2)
	started, continued := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(continued) })
	defer release()
	var first sync.Once
	harness.Pipe(testStack.harnessRuntime, atom.StageModelResponse, func(operationContext context.Context, message atom.Message) (atom.Message, error) {
		first.Do(func() {
			close(started)
			select {
			case <-continued:
			case <-operationContext.Done():
			}
		})
		return message, operationContext.Err()
	})
	queue := newTestQueue(test, testStack.loop)
	_, _, operationError := queue.Submit(context.Background(), session, "start")
	testutil.RequireNoError(test, operationError)
	awaitSignal(test, started)
	for _, text := range []string{"one", "two", "three"} {
		_, _, operationError = queue.Submit(context.Background(), session, text)
		testutil.RequireNoError(test, operationError)
	}
	release()
	awaitQueuedIdle(test, queue, session.ID)
	if testStack.provider.Calls() != 2 {
		test.Fatalf("calls = %d", testStack.provider.Calls())
	}
	var users []string
	for _, message := range testStack.provider.Requests[1].Messages {
		if message.Role == atom.RoleUser {
			users = append(users, message.Content[0].Text)
		}
	}
	if len(users) != 4 || users[1] != "one" || users[2] != "two" || users[3] != "three" {
		test.Fatalf("request did not include the batch: %v", users)
	}
}

func TestRestoredPendingMessagesShareTheFirstRequest(test *testing.T) {
	testStack := newStack(test, provider.Text("restored batch"))
	session := testStack.instance(test, 2)
	operationContext := context.Background()
	for _, text := range []string{"restored one", "restored two", "restored three"} {
		testutil.RequireNoError(test, testStack.database.Queue().Enqueue(operationContext, atom.Message{ID: text, SessionID: session.ID, Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: text}}, CreatedAt: time.Now()}, 128))
	}
	queue := newTestQueue(test, testStack.loop)
	testutil.RequireNoError(test, queue.Restore(operationContext))
	awaitQueuedIdle(test, queue, session.ID)
	if testStack.provider.Calls() != 1 {
		test.Fatalf("restored messages ran separately: %d", testStack.provider.Calls())
	}
	users := 0
	for _, message := range testStack.provider.Requests[0].Messages {
		if message.Role == atom.RoleUser {
			users++
		}
	}
	if users != 3 {
		test.Fatalf("restored request contains %d user messages", users)
	}
}

func TestNewQueuedInputCannotDisappearFromPreparedRequest(test *testing.T) {
	testStack := newStack(test, provider.Text("must not receive incomplete input"))
	session := testStack.instance(test, 2)
	harness.Pipe(testStack.harnessRuntime, atom.StageModelRequest, func(operationContext context.Context, request atom.Request) (atom.Request, error) {
		kept := request.Messages[:0]
		for _, message := range request.Messages {
			if message.Role != atom.RoleUser {
				kept = append(kept, message)
			}
		}
		request.Messages = kept
		return request, nil
	})
	queue := newTestQueue(test, testStack.loop)
	accepted, _, operationError := queue.Submit(context.Background(), session, "required new input")
	testutil.RequireNoError(test, operationError)
	deadline := time.Now().Add(3 * time.Second)
	for {
		status, readError := queue.Status(context.Background(), session.ID)
		testutil.RequireNoError(test, readError)
		if !status.Running && status.Error != "" {
			break
		}
		if time.Now().After(deadline) {
			test.Fatal("excluded input was not reported")
		}
		time.Sleep(time.Millisecond)
	}
	if testStack.provider.Calls() != 0 {
		test.Fatal("provider received a request without admitted input")
	}
	messages, operationError := testStack.database.Sessions().Messages(context.Background(), session.ID)
	testutil.RequireNoError(test, operationError)
	if len(messages) != 1 || messages[0].ID != accepted.ID {
		test.Fatal("excluded input was removed from saved history")
	}
}
