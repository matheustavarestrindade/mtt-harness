package taskstate_test

import (
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/taskstate"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestTaskUpdateRejectsMalformedAndOversizedInputs(test *testing.T) {
	for _, input := range []string{
		`null`, `{}`, `{"todo":null}`, `{"todo":[null]}`, `{"unknown":true}`, `{"doing":{}}`,
		`{"todo":[{"id":"1","title":null}]}`, `{"todo":[{"id":"1","status":"unknown"}]}`,
		`{"todo":[{"id":"1"},{"id":"1"}]}`, `{"todo":[{"id":"bad id"}]}`,
		`{"doing":{"title":" ","description":"something"}}`,
		`{"doing":{"title":"x","description":"` + strings.Repeat("x", 1201) + `"}}`,
	} {
		if _, operationError := taskstate.DecodeUpdate([]byte(input)); operationError == nil {
			test.Errorf("accepted invalid input: %s", input)
		}
	}
}

func TestTaskCancellationCompletionAndDoingOnlyUpdates(test *testing.T) {
	state := taskstate.Empty("session")
	for _, input := range []string{
		`{"todo":[{"id":"1","title":"First"},{"id":"2","title":"Second"}],"doing":{"title":"Work","description":"Current work."}}`,
		`{"todo":[{"id":"1","status":"done"}]}`,
	} {
		update, operationError := taskstate.DecodeUpdate([]byte(input))
		testutil.RequireNoError(test, operationError)
		state, operationError = taskstate.ApplyUpdate(state, update, time.Now())
		testutil.RequireNoError(test, operationError)
	}
	if len(state.Todo) != 2 || state.Todo[0].Status != atom.TaskDone {
		test.Fatal("completed item disappeared before the work finished")
	}
	update, operationError := taskstate.DecodeUpdate([]byte(`{"todo":[{"id":"2","status":"cancelled"}]}`))
	testutil.RequireNoError(test, operationError)
	state, operationError = taskstate.ApplyUpdate(state, update, time.Now())
	testutil.RequireNoError(test, operationError)
	if taskstate.Active(state) {
		test.Fatal("all terminal work was not cleared")
	}
	update, operationError = taskstate.DecodeUpdate([]byte(`{"doing":{"title":"Review","description":"One focused review."}}`))
	testutil.RequireNoError(test, operationError)
	state, operationError = taskstate.ApplyUpdate(state, update, time.Now())
	testutil.RequireNoError(test, operationError)
	if state.Doing == nil || len(state.Todo) != 0 {
		test.Fatal("activity required a checklist")
	}
}
