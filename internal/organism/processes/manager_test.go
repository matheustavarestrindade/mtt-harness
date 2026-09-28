package processes

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/eventbus"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/process"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

type testWatcher struct {
	events chan atom.ProcessEvent
}

func (testWatcher *testWatcher) Match(event atom.ProcessEvent) bool {
	return true
}

func (testWatcher *testWatcher) OnMatch(operationContext context.Context, event atom.ProcessEvent) {
	select {
	case testWatcher.events <- event:
	default:
	}
}

func TestManagerAppliesTheOutputStage(test *testing.T) {
	database := memory.New()
	harnessRuntime := harness.New()
	bus := eventbus.New(harnessRuntime)
	harness.Pipe(harnessRuntime, atom.StageProcessOutput, func(operationContext context.Context, event atom.ProcessEvent) (atom.ProcessEvent, error) {
		if event.Stream == atom.StreamStdout {
			event.Data = []byte(strings.ToUpper(string(event.Data)))
		}
		return event, nil
	})
	watcher := &testWatcher{events: make(chan atom.ProcessEvent, 16)}
	harnessRuntime.Watch(watcher)
	supervisor := process.New(4)
	manager := New(supervisor, database, harnessRuntime, bus, nil)
	session := atom.Session{ID: "session-1", InstanceID: "instance-1", CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Sessions().Save(context.Background(), session))

	operationContext := harness.WithSession(context.Background(), session)
	runningProcess, operationError := manager.Start(operationContext, atom.ProcessSpec{Command: "sh", Args: []string{"-c", "echo hello"}})
	testutil.RequireNoError(test, operationError)

	if _, operationError := runningProcess.Wait(); operationError != nil {
		test.Fatal(operationError)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case event := <-watcher.events:
			if event.Stream == atom.StreamStdout && strings.Contains(string(event.Data), "HELLO") {
				return
			}
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	test.Fatal("the transformed output did not arrive")
}

func TestManagerSendsNotification(test *testing.T) {
	database := memory.New()
	harnessRuntime := harness.New()
	bus := eventbus.New(harnessRuntime)
	supervisor := process.New(4)
	manager := New(supervisor, database, harnessRuntime, bus, nil)
	session := atom.Session{ID: "session-1", InstanceID: "instance-1", CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Sessions().Save(context.Background(), session))

	operationContext := harness.WithSession(context.Background(), session)
	runningProcess, operationError := manager.Start(operationContext, atom.ProcessSpec{
		Command: "sh",
		Args:    []string{"-c", "echo ready"},
		Notify:  atom.NotifyPolicy{Mode: atom.NotifyExit},
	})
	testutil.RequireNoError(test, operationError)

	if _, operationError := runningProcess.Wait(); operationError != nil {
		test.Fatal(operationError)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		messages, _ := database.Sessions().Messages(context.Background(), session.ID)
		if len(messages) > 0 && strings.Contains(messages[0].Content[0].Text, "[process]") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	test.Fatal("the notification message did not arrive")
}
