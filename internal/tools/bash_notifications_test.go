package tools

import (
	"context"
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

func TestForegroundBashOwnsProcessOutputBeforeStarting(test *testing.T) {
	for _, input := range []string{
		`{"command":"printf result"}`,
		`{"command":"printf result","wait":true,"notify":"exit"}`,
		`{"command":"printf result","wait":true,"notify":"interval","interval":1}`,
	} {
		test.Run(input, func(test *testing.T) {
			database := memory.New()
			harnessRuntime := harness.New()
			processManager := processes.New(process.New(2), database, harnessRuntime, eventbus.New(harnessRuntime), nil)
			test.Cleanup(func() { testutil.RequireNoError(test, processManager.Close(context.Background())) })
			session := atom.Session{ID: "foreground", InstanceID: "instance", CreatedAt: time.Now()}
			testutil.RequireNoError(test, database.Sessions().Save(context.Background(), session))
			operationContext := harness.WithWorkspace(harness.WithSession(context.Background(), session), test.TempDir())
			result, operationError := NewBash(processManager).Run(operationContext, atom.ToolCall{ID: "bash-call", Input: []byte(input)})
			testutil.RequireNoError(test, operationError)
			if result.CallID != "bash-call" || result.Status != atom.StatusOK || result.Text() != "result" {
				test.Fatalf("foreground tool result: %+v", result)
			}
			records, operationError := database.Processes().List(context.Background(), session.ID)
			testutil.RequireNoError(test, operationError)
			if len(records) != 1 || records[0].Spec.Notify.Mode != atom.NotifyNone {
				test.Fatalf("foreground notification policy was not set before Start: %+v", records)
			}
		})
	}
}
