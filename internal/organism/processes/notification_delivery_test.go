package processes

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/eventbus"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/process"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestNoNotificationPolicyStillRecordsExitAndOutput(test *testing.T) {
	database := memory.New()
	harnessRuntime := harness.New()
	processManager := New(process.New(2), database, harnessRuntime, eventbus.New(harnessRuntime), nil)
	test.Cleanup(func() { testutil.RequireNoError(test, processManager.Close(context.Background())) })
	var delivered atomic.Int32
	processManager.SetNotifier(func(context.Context, atom.Session, []atom.Content) error { delivered.Add(1); return nil })
	session := atom.Session{ID: "foreground", InstanceID: "instance"}
	testutil.RequireNoError(test, database.Sessions().Save(context.Background(), session))
	operationContext := harness.WithSession(context.Background(), session)
	runningProcess, operationError := processManager.Start(operationContext, atom.ProcessSpec{Command: "sh", Args: []string{"-c", "printf output; exit 7"}, Notify: atom.NotifyPolicy{Mode: atom.NotifyNone}})
	testutil.RequireNoError(test, operationError)
	_, _ = runningProcess.Wait()
	// Start has registered the only observer before returning. Join it without
	// closing the manager, which would independently suppress late notifications.
	processManager.observers.Wait()
	if delivered.Load() != 0 {
		test.Fatal("a completed foreground process sent a duplicate notification")
	}
	records, operationError := database.Processes().List(context.Background(), session.ID)
	testutil.RequireNoError(test, operationError)
	if len(records) != 1 || records[0].Status != "stopped" || records[0].Exit == nil || records[0].Exit.Code != 7 {
		test.Fatalf("process exit was lost: %+v", records)
	}
	stdout, _, retained := processManager.Output(runningProcess.ID())
	if !retained || string(stdout) != "output" {
		test.Fatalf("retained output was lost: %q", stdout)
	}
}

func TestBackgroundNotificationHasRuntimeOrigin(test *testing.T) {
	database := memory.New()
	harnessRuntime := harness.New()
	processManager := New(process.New(2), database, harnessRuntime, eventbus.New(harnessRuntime), nil)
	test.Cleanup(func() { testutil.RequireNoError(test, processManager.Close(context.Background())) })
	session := atom.Session{ID: "background", InstanceID: "instance", CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Sessions().Save(context.Background(), session))
	operationContext := harness.WithSession(context.Background(), session)
	runningProcess, operationError := processManager.Start(operationContext, atom.ProcessSpec{Command: "sh", Args: []string{"-c", "printf background"}, Notify: atom.NotifyPolicy{Mode: atom.NotifyExit}})
	testutil.RequireNoError(test, operationError)
	_, _ = runningProcess.Wait()
	processManager.observers.Wait()
	messages, operationError := database.Sessions().Messages(context.Background(), session.ID)
	testutil.RequireNoError(test, operationError)
	if len(messages) != 1 || messages[0].Role != atom.RoleRuntime || !strings.Contains(messages[0].Content[0].Text, "not a new user request") || !strings.Contains(messages[0].Content[0].Text, "background") {
		test.Fatalf("background output was attributed incorrectly: %+v", messages)
	}
}
