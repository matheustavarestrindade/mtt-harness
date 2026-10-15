package processes

import (
	"context"
	"errors"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/process"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestUnlinkGateProtectsProcessStartsWithoutKillingProcesses(test *testing.T) {
	operationContext := context.Background()
	database := memory.New()
	manager := New(process.New(8), database, harness.New(), nil, nil)
	defer manager.Close(operationContext)
	releaseReservation, operationError := manager.reserveStart(operationContext, "workspace")
	testutil.RequireNoError(test, operationError)
	if _, operationError = manager.FenceInstanceUnlink("workspace"); !errors.Is(operationError, store.ErrInstanceBusy) {
		test.Fatal("unlink passed a process launch reservation")
	}
	releaseReservation()
	session := atom.Session{ID: "process-session", InstanceID: "workspace"}
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	processContext := harness.WithSession(operationContext, session)
	running, operationError := manager.Start(processContext, atom.ProcessSpec{Command: "sh", Args: []string{"-c", "sleep 20"}, Notify: atom.NotifyPolicy{Mode: atom.NotifyNone}})
	testutil.RequireNoError(test, operationError)
	if _, operationError = manager.FenceInstanceUnlink("workspace"); !errors.Is(operationError, store.ErrInstanceBusy) {
		test.Fatal("unlink accepted an active process")
	}
	if !running.(*process.Handle).Running() {
		test.Fatal("unlink stopped a process")
	}
	testutil.RequireNoError(test, running.Kill(""))
	_, _ = running.Wait()
	finish, operationError := manager.FenceInstanceUnlink("workspace")
	testutil.RequireNoError(test, operationError)
	if _, operationError = manager.Start(processContext, atom.ProcessSpec{Command: "true"}); !errors.Is(operationError, store.ErrInstanceBusy) {
		test.Fatal("new process bypassed unlink fence")
	}
	finish(false)
	resumed, operationError := manager.Start(processContext, atom.ProcessSpec{Command: "true", Notify: atom.NotifyPolicy{Mode: atom.NotifyNone}})
	testutil.RequireNoError(test, operationError)
	_, operationError = resumed.Wait()
	testutil.RequireNoError(test, operationError)
	finish, operationError = manager.FenceInstanceUnlink("workspace")
	testutil.RequireNoError(test, operationError)
	finish(true)
	if _, operationError = manager.Start(processContext, atom.ProcessSpec{Command: "true"}); !errors.Is(operationError, store.ErrInstanceUnlinked) {
		test.Fatal("committed unlink fence reopened")
	}
}
