package process

import (
	"context"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestSupervisorStreamsOutput(test *testing.T) {
	supervisor := New(4)
	runningProcess, operationError := supervisor.Start(context.Background(), atom.ProcessSpec{
		Command: "sh",
		Args:    []string{"-c", "echo hello; echo oops >&2; exit 3"},
	})
	testutil.RequireNoError(test, operationError)

	status, operationError := runningProcess.Wait()
	if operationError == nil {
		test.Fatal("the process does not give an error")
	}
	if status.Code != 3 {
		test.Fatalf("exit code = %d", status.Code)
	}
	handle := runningProcess.(*Handle)
	stdout, stderr := handle.Output()
	if !strings.Contains(string(stdout), "hello") {
		test.Fatalf("stdout = %q", stdout)
	}
	if !strings.Contains(string(stderr), "oops") {
		test.Fatalf("stderr = %q", stderr)
	}
}

func TestSupervisorStopsAfterTimeout(test *testing.T) {
	supervisor := New(4)
	runningProcess, operationError := supervisor.Start(context.Background(), atom.ProcessSpec{
		Command: "sh",
		Args:    []string{"-c", "sleep 5"},
		Timeout: 100 * 1e6,
	})
	testutil.RequireNoError(test, operationError)

	status, operationError := runningProcess.Wait()
	if operationError == nil {
		test.Fatal("the process does not give an error")
	}
	if status.Code == 0 {
		test.Fatalf("the process is not stopped: %+v", status)
	}
}
