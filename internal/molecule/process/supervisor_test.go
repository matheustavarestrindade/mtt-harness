package process

import (
	"context"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func TestSupervisorStreamsOutput(t *testing.T) {
	supervisor := New(4)
	proc, err := supervisor.Start(context.Background(), atom.ProcessSpec{
		Command: "sh",
		Args:    []string{"-c", "echo hello; echo oops >&2; exit 3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	status, err := proc.Wait()
	if err == nil {
		t.Fatal("the process does not give an error")
	}
	if status.Code != 3 {
		t.Fatalf("exit code = %d", status.Code)
	}
	handle := proc.(*Handle)
	stdout, stderr := handle.Output()
	if !strings.Contains(string(stdout), "hello") {
		t.Fatalf("stdout = %q", stdout)
	}
	if !strings.Contains(string(stderr), "oops") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestSupervisorStopsAfterTimeout(t *testing.T) {
	supervisor := New(4)
	proc, err := supervisor.Start(context.Background(), atom.ProcessSpec{
		Command: "sh",
		Args:    []string{"-c", "sleep 5"},
		Timeout: 100 * 1e6,
	})
	if err != nil {
		t.Fatal(err)
	}
	status, err := proc.Wait()
	if err == nil {
		t.Fatal("the process does not give an error")
	}
	if status.Code == 0 {
		t.Fatalf("the process is not stopped: %+v", status)
	}
}
