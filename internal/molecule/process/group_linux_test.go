package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestKillTerminatesShellDescendants(test *testing.T) {
	supervisor := New(1)
	runningProcess, operationError := supervisor.Start(context.Background(), atom.ProcessSpec{Command: "sh", Args: []string{"-c", "sleep 30 & echo $!; wait"}})
	testutil.RequireNoError(test, operationError)
	defer runningProcess.Kill("")
	operationContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	events := runningProcess.(*Handle).SubscribeContext(operationContext)
	var childPID int
	for childPID == 0 {
		select {
		case event := <-events:
			if event.Stream == atom.StreamStdout {
				childPID, _ = strconv.Atoi(strings.TrimSpace(string(event.Data)))
			}
		case <-operationContext.Done():
			test.Fatal("child PID did not arrive")
		}
	}
	testutil.RequireNoError(test, runningProcess.Kill(""))
	finished := make(chan struct{})
	go func() {
		runningProcess.Wait()
		close(finished)
	}()
	select {
	case <-finished:
	case <-operationContext.Done():
		test.Fatal("a descendant kept output pipes open after kill")
	}
	data, operationError := os.ReadFile(fmt.Sprintf("/proc/%d/stat", childPID))
	// Linux can return ESRCH while a disappearing /proc entry is being read,
	// as well as ENOENT at open time. Both establish that the child is gone.
	if os.IsNotExist(operationError) || errors.Is(operationError, syscall.ESRCH) {
		return
	}
	testutil.RequireNoError(test, operationError)
	fields := strings.Fields(string(data))
	if len(fields) > 2 && fields[2] == "Z" {
		return
	}
	test.Fatalf("descendant still running: %s", data)
}
