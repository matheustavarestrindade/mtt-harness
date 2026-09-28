package loop_test

import (
	"context"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestQueueRunsMessagesInSequence(test *testing.T) {
	testStack := newStack(test, provider.Text("first"), provider.Text("second"))
	session := testStack.instance(test, 2)
	queue := loop.NewQueue(testStack.loop)
	operationContext := context.Background()
	if _, position, operationError := queue.Submit(operationContext, session, "one"); operationError != nil || position != 1 {
		test.Fatalf("submit one = %d %v", position, operationError)
	}
	if _, position, operationError := queue.Submit(operationContext, session, "two"); operationError != nil || position < 1 || position > 2 {
		test.Fatalf("submit two = %d %v", position, operationError)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		messages, _ := testStack.database.Sessions().Messages(operationContext, session.ID)
		if len(messages) >= 4 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	messages, _ := testStack.database.Sessions().Messages(operationContext, session.ID)
	if len(messages) != 4 {
		test.Fatalf("the messages = %d", len(messages))
	}
	if messages[1].Content[0].Text != "first" || messages[3].Content[0].Text != "second" {
		test.Fatalf("the sequence is not correct: %q %q", messages[1].Content[0].Text, messages[3].Content[0].Text)
	}
	if testStack.provider.Calls() != 2 {
		test.Fatalf("the model calls = %d", testStack.provider.Calls())
	}
}

func TestQueueCancelStopsTheRun(test *testing.T) {
	testStack := newStack(test, provider.Text("slow"))
	testStack.provider.SetDelay(2 * time.Second)
	session := testStack.instance(test, 2)
	queue := loop.NewQueue(testStack.loop)
	operationContext := context.Background()
	if _, _, operationError := queue.Submit(operationContext, session, "one"); operationError != nil {
		test.Fatal(operationError)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if running, _ := queue.Status(session.ID); running {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if running, _ := queue.Status(session.ID); !running {
		test.Fatal("the run did not start")
	}
	if !queue.Cancel(session.ID) {
		test.Fatal("the run is not cancelled")
	}
	for time.Now().Before(deadline) {
		if running, _ := queue.Status(session.ID); !running {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	messages, _ := testStack.database.Sessions().Messages(operationContext, session.ID)
	if len(messages) != 1 {
		test.Fatalf("the messages = %d", len(messages))
	}
}

func TestQueueCancelsAQueuedMessage(test *testing.T) {
	testStack := newStack(test, provider.Text("first"), provider.Text("third"))
	testStack.provider.SetDelay(200 * time.Millisecond)
	session := testStack.instance(test, 2)
	queue := loop.NewQueue(testStack.loop)
	operationContext := context.Background()
	if _, _, operationError := queue.Submit(operationContext, session, "one"); operationError != nil {
		test.Fatal(operationError)
	}
	second, _, operationError := queue.Submit(operationContext, session, "two")
	testutil.RequireNoError(test, operationError)

	if _, _, operationError := queue.Submit(operationContext, session, "three"); operationError != nil {
		test.Fatal(operationError)
	}
	removed, operationError := queue.CancelMessage(operationContext, session.ID, second.ID)
	testutil.RequireNoError(test, operationError)
	if !removed {
		test.Fatal("the queued message is not removed")
	}
	removed, operationError = queue.CancelMessage(operationContext, session.ID, "not-there")
	testutil.RequireNoError(test, operationError)
	if removed {
		test.Fatal("a message which is not in the queue is removed")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		messages, _ := testStack.database.Sessions().Messages(operationContext, session.ID)
		if len(messages) >= 4 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	messages, _ := testStack.database.Sessions().Messages(operationContext, session.ID)
	if len(messages) != 4 {
		test.Fatalf("the messages = %d", len(messages))
	}
	if messages[0].Content[0].Text != "one" || messages[2].Content[0].Text != "three" {
		test.Fatalf("the removed message ran: %q %q", messages[0].Content[0].Text, messages[2].Content[0].Text)
	}
	for _, message := range messages {
		if message.Content[0].Text == "two" {
			test.Fatal("the removed message is in the store")
		}
	}
}
