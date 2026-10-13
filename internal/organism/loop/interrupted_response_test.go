package loop_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

type interruptedProvider struct{ *provider.Test }
type interruptedStream struct{ harness.Stream }

func (modelProvider interruptedProvider) Stream(operationContext context.Context, request atom.Request) (harness.Stream, error) {
	stream, operationError := modelProvider.Test.Stream(operationContext, request)
	return interruptedStream{stream}, operationError
}
func (stream interruptedStream) Recv(operationContext context.Context) (atom.ResponsePart, error) {
	part, operationError := stream.Stream.Recv(operationContext)
	if errors.Is(operationError, io.EOF) {
		return part, errors.New("generation interrupted: aborted")
	}
	return part, operationError
}

func TestInterruptedResponseRetainsPartialTextAndReportedCharge(test *testing.T) {
	testStack := newStack(test, provider.TextWithUsage("partial answer", atom.Usage{Input: 10, Output: 3, Cost: &atom.Cost{Currency: "USD", Value: 0.7}}))
	testStack.harnessRuntime.Provider(interruptedProvider{testStack.provider})
	session := testStack.instance(test, 2)
	testStack.user(test, session, "start")
	operationError := testStack.loop.Run(context.Background(), session)
	if operationError == nil || !strings.Contains(operationError.Error(), "aborted") {
		test.Fatalf("interrupted response became successful: %v", operationError)
	}
	history, operationError := testStack.database.Sessions().Messages(context.Background(), session.ID)
	testutil.RequireNoError(test, operationError)
	if len(history) != 2 || history[1].Content[0].Text != "partial answer" || history[1].Usage == nil || history[1].Usage.Cost.Value != 0.7 {
		test.Fatalf("partial history lost: %+v", history)
	}
	total, operationError := testStack.database.Usage().Session(context.Background(), session.ID)
	testutil.RequireNoError(test, operationError)
	if total.Calls != 1 || len(total.Costs) != 1 || total.Costs[0].Value != 0.7 || total.Costs[0].Estimated {
		test.Fatalf("interrupted call was not billed accurately: %+v", total)
	}
	state, operationError := testStack.database.TaskStates().Get(context.Background(), session.ID)
	testutil.RequireNoError(test, operationError)
	if state.Doing != nil {
		test.Fatal("interrupted response retained DOING")
	}
}
