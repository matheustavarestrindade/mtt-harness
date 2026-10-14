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

func TestBackgroundInactivityNoticeIsOnceAndExitRemainsSeparate(test *testing.T) {
	database := memory.New()
	runtime := harness.New()
	manager := New(process.New(2), database, runtime, eventbus.New(runtime), nil)
	test.Cleanup(func() { testutil.RequireNoError(test, manager.Close(context.Background())) })
	session := atom.Session{ID: "background-idle", InstanceID: "instance"}
	testutil.RequireNoError(test, database.Sessions().Save(context.Background(), session))
	operationContext := harness.WithSession(context.Background(), session)
	notifications := make(chan string, 8)
	manager.SetNotifier(func(_ context.Context, _ atom.Session, content []atom.Content) error {
		notifications <- content[0].Text
		return nil
	})
	runningProcess, operationError := manager.Start(operationContext, atom.ProcessSpec{Command: "sh", Args: []string{"-c", "printf first; read one; printf second; read two"}, Notify: atom.NotifyPolicy{Mode: atom.NotifyExit}, IdleTimeout: 50 * time.Millisecond})
	testutil.RequireNoError(test, operationError)
	select {
	case text := <-notifications:
		if !strings.Contains(text, "state=running") || !strings.Contains(text, "first") {
			test.Fatalf("notice: %s", text)
		}
	case <-time.After(2 * time.Second):
		test.Fatal("no inactivity notice")
	}
	testutil.RequireNoError(test, runningProcess.Write([]byte("first\n")))
	select {
	case text := <-notifications:
		test.Fatalf("repeated inactivity notice: %s", text)
	case <-time.After(150 * time.Millisecond):
	}
	testutil.RequireNoError(test, runningProcess.Write([]byte("second\n")))
	_, operationError = runningProcess.Wait()
	testutil.RequireNoError(test, operationError)
	manager.observers.Wait()
	select {
	case text := <-notifications:
		if !strings.Contains(text, "stopped") {
			test.Fatalf("missing exit: %s", text)
		}
	default:
		test.Fatal("missing exit notice")
	}
	select {
	case text := <-notifications:
		test.Fatalf("extra notice: %s", text)
	default:
	}
}

func TestForegroundReleaseAfterExitStillNotifiesOnceAndCloseJoins(test *testing.T) {
	for _, releasePolicy := range []bool{false, true} {
		test.Run(map[bool]string{false: "unreleased-close", true: "late-background"}[releasePolicy], func(test *testing.T) {
			database := memory.New()
			runtime := harness.New()
			manager := New(process.New(2), database, runtime, eventbus.New(runtime), nil)
			session := atom.Session{ID: "late", InstanceID: "instance"}
			testutil.RequireNoError(test, database.Sessions().Save(context.Background(), session))
			runningProcess, release, operationError := manager.StartForeground(harness.WithSession(context.Background(), session), atom.ProcessSpec{Command: "sh", Args: []string{"-c", "printf done"}})
			testutil.RequireNoError(test, operationError)
			_, operationError = runningProcess.Wait()
			testutil.RequireNoError(test, operationError)
			if releasePolicy {
				release(atom.NotifyPolicy{Mode: atom.NotifyExit})
				release(atom.NotifyPolicy{Mode: atom.NotifyNone})
				manager.observers.Wait()
			}
			closeContext, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			testutil.RequireNoError(test, manager.Close(closeContext))
			messages, operationError := database.Sessions().Messages(context.Background(), session.ID)
			testutil.RequireNoError(test, operationError)
			if releasePolicy && (len(messages) != 1 || messages[0].Role != atom.RoleRuntime) {
				test.Fatalf("late handoff messages: %+v", messages)
			}
		})
	}
}

func TestInactivityOutputIsBoundedAndPreservesLiteralText(test *testing.T) {
	text := []byte(strings.Repeat("line\n", 100) + "\x1b[36mfinal\x1b[0m\n")
	tail := processOutputTail(text)
	if strings.Count(tail, "\n") > 20 || !strings.HasSuffix(tail, "\x1b[36mfinal\x1b[0m\n") {
		test.Fatalf("tail changed output: %q", tail)
	}
	if len(processOutputTail([]byte(strings.Repeat("x", 10000)))) > 4096 {
		test.Fatal("tail exceeds byte bound")
	}
}
