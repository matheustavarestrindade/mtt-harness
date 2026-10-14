package process

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestOutputActivityPostponesInactivityAndSnapshotsKeepByteOffsets(test *testing.T) {
	processSupervisor := New(2)
	test.Cleanup(func() { testutil.RequireNoError(test, processSupervisor.Close(context.Background())) })
	runningProcess, operationError := processSupervisor.Start(context.Background(), atom.ProcessSpec{Command: "sh", Args: []string{"-c", "printf first; read one; printf second; read two"}})
	testutil.RequireNoError(test, operationError)
	processHandle := runningProcess.(*Handle)
	operationContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events := processHandle.SubscribeContext(operationContext)
	for event := range events {
		if event.Stream == atom.StreamStdout && strings.Contains(string(event.Data), "first") {
			break
		}
	}
	_, observation, operationError := processHandle.WaitForExitOrIdle(operationContext, 40*time.Millisecond)
	testutil.RequireNoError(test, operationError)
	if observation == nil || !observation.Running || string(observation.Stdout) != "first" {
		test.Fatalf("inactivity observation: %+v", observation)
	}
	testutil.RequireNoError(test, runningProcess.Write([]byte("second\n")))
	for event := range events {
		if event.Stream == atom.StreamStdout && strings.Contains(string(event.Data), "second") {
			break
		}
	}
	_, later, operationError := processHandle.WaitForExitOrIdle(operationContext, 40*time.Millisecond)
	testutil.RequireNoError(test, operationError)
	if later == nil || !later.LastOutputAt.After(observation.LastOutputAt) || string(later.Stdout) != "firstsecond" {
		test.Fatalf("new output did not move activity: %+v", later)
	}
	// Middleware-sized writes may exceed retention; the cursor still counts all
	// published bytes so a reader can distinguish truncation from missing events.
	_, operationError = (outputWriter{handle: processHandle, stream: atom.StreamStdout}).Write([]byte(strings.Repeat("x", outputLimit+10)))
	testutil.RequireNoError(test, operationError)
	snapshot := processHandle.SubscribeContext(operationContext)
	for event := range snapshot {
		if event.Stream == atom.StreamStdout {
			if !event.Snapshot || event.Offset != 21 || len(event.Data) != outputLimit {
				test.Fatalf("snapshot offset: %+v", event)
			}
			break
		}
	}
	testutil.RequireNoError(test, runningProcess.Write([]byte("exit\n")))
	_, operationError = runningProcess.Wait()
	testutil.RequireNoError(test, operationError)
}
