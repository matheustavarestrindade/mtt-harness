package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/eventbus"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/process"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/processes"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestForegroundInactivityReturnsLiveProcessAndOneExitNotification(test *testing.T) {
	database := memory.New()
	runtime := harness.New()
	manager := processes.New(process.New(2), database, runtime, eventbus.New(runtime), nil)
	test.Cleanup(func() { testutil.RequireNoError(test, manager.Close(context.Background())) })
	session := atom.Session{ID: "idle-session", InstanceID: "instance"}
	testutil.RequireNoError(test, database.Sessions().Save(context.Background(), session))
	operationContext := harness.WithToolOrigin(harness.WithWorkspace(harness.WithSession(context.Background(), session), test.TempDir()), harness.ToolOrigin{MessageID: "response", CallID: "call"})
	notifications := make(chan string, 8)
	manager.SetNotifier(func(_ context.Context, _ atom.Session, content []atom.Content) error {
		notifications <- content[0].Text
		return nil
	})
	input, _ := json.Marshal(map[string]any{"command": "printf '[info] Sync finished in 0.123s\\n'; read reply; printf 'real exit\\n'", "idle_timeout": 80, "timeout": 5000})
	result, operationError := NewBash(manager).Run(operationContext, atom.ToolCall{ID: "call", Input: input})
	testutil.RequireNoError(test, operationError)
	if !strings.Contains(result.Text(), "state=running") || !strings.Contains(result.Text(), "[info] Sync finished in 0.123s") {
		test.Fatalf("missing factual inactivity result: %+v", result)
	}
	records, operationError := database.Processes().List(context.Background(), session.ID)
	testutil.RequireNoError(test, operationError)
	if len(records) != 1 || records[0].Spec.ToolMessageID != "response" || records[0].Spec.ToolCallID != "call" {
		test.Fatalf("lost process origin: %+v", records)
	}
	runningProcess, found := manager.Get(records[0].ID)
	if !found || !runningProcess.(*process.Handle).Running() {
		test.Fatal("inactivity stopped the command")
	}
	select {
	case value := <-notifications:
		test.Fatalf("duplicated foreground notice: %s", value)
	default:
	}
	testutil.RequireNoError(test, runningProcess.Write([]byte("continue\n")))
	_, operationError = runningProcess.Wait()
	testutil.RequireNoError(test, operationError)
	select {
	case value := <-notifications:
		if !strings.Contains(value, "stopped") || !strings.Contains(value, "real exit") {
			test.Fatalf("missing real exit: %s", value)
		}
	case <-time.After(2 * time.Second):
		test.Fatal("missing background exit notification")
	}
	testutil.RequireNoError(test, manager.Close(context.Background()))
	select {
	case value := <-notifications:
		test.Fatalf("duplicate notification: %s", value)
	default:
	}
}

func TestZeroInactivityWaitAndCancellationDoNotEnableNotifications(test *testing.T) {
	for _, cancelWait := range []bool{false, true} {
		test.Run(map[bool]string{false: "exit", true: "cancel"}[cancelWait], func(test *testing.T) {
			database := memory.New()
			runtime := harness.New()
			manager := processes.New(process.New(2), database, runtime, eventbus.New(runtime), nil)
			test.Cleanup(func() { testutil.RequireNoError(test, manager.Close(context.Background())) })
			session := atom.Session{ID: "quiet", InstanceID: "instance"}
			testutil.RequireNoError(test, database.Sessions().Save(context.Background(), session))
			operationContext, cancel := context.WithCancel(harness.WithWorkspace(harness.WithSession(context.Background(), session), test.TempDir()))
			defer cancel()
			results := make(chan atom.ToolResult, 1)
			go func() {
				result, _ := NewBash(manager).Run(operationContext, atom.ToolCall{ID: "quiet", Input: []byte(`{"command":"read reply; printf done","idle_timeout":0,"terminal":false}`)})
				results <- result
			}()
			var runningProcess harness.Process
			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) {
				if all := manager.All(); len(all) > 0 {
					runningProcess = all[0]
					break
				}
				time.Sleep(time.Millisecond)
			}
			if runningProcess == nil {
				test.Fatal("process did not start")
			}
			select {
			case result := <-results:
				test.Fatalf("quiet wait returned early: %+v", result)
			case <-time.After(100 * time.Millisecond):
			}
			if cancelWait {
				cancel()
				select {
				case result := <-results:
					if result.Status != atom.StatusError {
						test.Fatalf("cancellation status: %+v", result)
					}
				case <-time.After(time.Second):
					test.Fatal("wait did not cancel")
				}
			}
			testutil.RequireNoError(test, runningProcess.Write([]byte("finish\n")))
			_, operationError := runningProcess.Wait()
			testutil.RequireNoError(test, operationError)
			if !cancelWait {
				select {
				case result := <-results:
					if result.Text() != "done" {
						test.Fatalf("exit result: %+v", result)
					}
				case <-time.After(time.Second):
					test.Fatal("wait did not finish")
				}
			}
			testutil.RequireNoError(test, manager.Close(context.Background()))
			messages, operationError := database.Sessions().Messages(context.Background(), session.ID)
			testutil.RequireNoError(test, operationError)
			if len(messages) != 0 {
				test.Fatalf("unexpected foreground runtime messages: %+v", messages)
			}
		})
	}
}
