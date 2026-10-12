package eventbus

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func TestSlowSessionPersistenceDoesNotBlockAnotherSession(test *testing.T) {
	eventBus := New(harness.New())
	entered, release := make(chan struct{}), make(chan struct{})
	var sequence atomic.Uint64
	eventBus.SetRecorder(func(operationContext context.Context, event atom.Event) (atom.Event, error) {
		if event.SessionID == "slow" {
			close(entered)
			<-release
		}
		event.Seq = sequence.Add(1)
		return event, nil
	})
	finished := make(chan error, 1)
	go func() { finished <- eventBus.Send(context.Background(), atom.Event{SessionID: "slow"}) }()
	<-entered
	other := make(chan error, 1)
	go func() { other <- eventBus.Send(context.Background(), atom.Event{SessionID: "other"}) }()
	select {
	case operationError := <-other:
		if operationError != nil {
			test.Fatal(operationError)
		}
	case <-time.After(time.Second):
		close(release)
		test.Fatal("unrelated session waited for the recorder")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if operationError := eventBus.Send(cancelled, atom.Event{SessionID: "slow"}); !errors.Is(operationError, context.Canceled) {
		test.Fatalf("cancelled admission: %v", operationError)
	}
	close(release)
	if operationError := <-finished; operationError != nil {
		test.Fatal(operationError)
	}
	if eventBus.LastSequence() != 2 || len(eventBus.sessions) != 0 {
		test.Fatal("event sequence or gate cleanup failed")
	}
}

func TestSameSessionGateHonorsDeadlineAndCallbackReentry(test *testing.T) {
	runtime := harness.New()
	eventBus := New(runtime)
	entered, release := make(chan struct{}), make(chan struct{})
	eventBus.SetRecorder(func(operationContext context.Context, event atom.Event) (atom.Event, error) {
		if event.Name == "first" {
			close(entered)
			<-release
		}
		return event, nil
	})
	finished := make(chan error, 1)
	go func() {
		finished <- eventBus.Send(context.Background(), atom.Event{SessionID: "session", Name: "first"})
	}()
	<-entered
	operationContext, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if operationError := eventBus.Send(operationContext, atom.Event{SessionID: "session"}); !errors.Is(operationError, context.DeadlineExceeded) {
		test.Fatalf("queued deadline: %v", operationError)
	}
	close(release)
	<-finished
	runtime.On("outer", func(operationContext context.Context, event atom.Event) {
		if operationError := eventBus.Send(operationContext, atom.Event{SessionID: event.SessionID, Name: "inner"}); operationError != nil {
			test.Error(operationError)
		}
	})
	if operationError := eventBus.Send(context.Background(), atom.Event{SessionID: "session", Name: "outer"}); operationError != nil {
		test.Fatal(operationError)
	}
	if len(eventBus.sessions) != 0 {
		test.Fatal("completed admissions retained gates")
	}
}
