package process

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestCompletedOutputAndTerminalReplay(test *testing.T) {
	supervisor := New(1)
	runningProcess, operationError := supervisor.Start(context.Background(), atom.ProcessSpec{Command: "sh", Args: []string{"-c", "printf first; printf error >&2; printf last"}})
	testutil.RequireNoError(test, operationError)
	_, operationError = runningProcess.Wait()
	testutil.RequireNoError(test, operationError)
	retained, found := supervisor.Get(runningProcess.ID())
	if !found {
		test.Fatal("completed output disappeared")
	}
	stdout, stderr := retained.(*Handle).Output()
	if string(stdout) != "firstlast" || string(stderr) != "error" {
		test.Fatalf("output was not drained: %q %q", stdout, stderr)
	}
	operationContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	events := retained.(*Handle).SubscribeContext(operationContext)
	exits := 0
	for {
		select {
		case event, open := <-events:
			if !open {
				if exits != 1 {
					test.Fatalf("exit replay count=%d", exits)
				}
				return
			}
			if event.Stream == atom.StreamExit {
				exits++
			}
		case <-operationContext.Done():
			test.Fatal("late subscription never closed")
		}
	}
}

func TestTransformAppliesBeforeBuffering(test *testing.T) {
	supervisor := New(1)
	runningProcess, operationError := supervisor.StartWithTransform(context.Background(), atom.ProcessSpec{Command: "sh", Args: []string{"-c", "printf secret"}}, func(event atom.ProcessEvent) (atom.ProcessEvent, error) {
		event.Data = []byte(strings.ReplaceAll(string(event.Data), "secret", "redacted"))
		return event, nil
	})
	testutil.RequireNoError(test, operationError)
	_, operationError = runningProcess.Wait()
	testutil.RequireNoError(test, operationError)
	stdout, _ := runningProcess.(*Handle).Output()
	if string(stdout) != "redacted" {
		test.Fatalf("raw output bypassed pipeline: %q", stdout)
	}
}
