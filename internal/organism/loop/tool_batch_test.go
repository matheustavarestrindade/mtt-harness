package loop_test

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

type streamedBatchProvider struct {
	fastStarted    chan struct{}
	slowStarted    chan struct{}
	releaseStream  chan struct{}
	completedTools atomic.Int32
	modelCalls     atomic.Int32
	requests       chan atom.Request
}

func (modelProvider *streamedBatchProvider) Name() string { return "batch" }
func (modelProvider *streamedBatchProvider) Models() []atom.ModelInfo {
	return []atom.ModelInfo{{ID: "batch-model", Input: []atom.MediaType{atom.Text}, Tools: true}}
}
func (modelProvider *streamedBatchProvider) Stream(operationContext context.Context, request atom.Request) (harness.Stream, error) {
	modelProvider.requests <- request
	if modelProvider.modelCalls.Add(1) == 1 {
		return &streamedBatchResponse{provider: modelProvider}, nil
	}
	if modelProvider.completedTools.Load() != 2 {
		return nil, fmt.Errorf("model was called before the complete tool batch finished")
	}
	return &completedBatchResponse{}, nil
}

type streamedBatchResponse struct {
	provider *streamedBatchProvider
	step     int
}

func (responseStream *streamedBatchResponse) Recv(operationContext context.Context) (atom.ResponsePart, error) {
	responseStream.step++
	if responseStream.step == 1 {
		index := 1
		return atom.ResponsePart{ToolCall: &atom.ToolCall{ID: "fast-call", Name: "fast", Input: []byte(`{}`)}, ToolIndex: &index}, nil
	}
	if responseStream.step == 2 {
		select {
		case <-responseStream.provider.fastStarted:
			index := 0
			return atom.ResponsePart{ToolCall: &atom.ToolCall{ID: "slow-call", Name: "slow", Input: []byte(`{}`)}, ToolIndex: &index}, nil
		case <-operationContext.Done():
			return atom.ResponsePart{}, operationContext.Err()
		}
	}
	select {
	case <-responseStream.provider.slowStarted:
	case <-operationContext.Done():
		return atom.ResponsePart{}, operationContext.Err()
	}
	select {
	case <-responseStream.provider.releaseStream:
		return atom.ResponsePart{}, io.EOF
	case <-operationContext.Done():
		return atom.ResponsePart{}, operationContext.Err()
	}
}

type completedBatchResponse struct{ delivered bool }

func (responseStream *completedBatchResponse) Recv(context.Context) (atom.ResponsePart, error) {
	if responseStream.delivered {
		return atom.ResponsePart{}, io.EOF
	}
	responseStream.delivered = true
	return atom.ResponsePart{Text: "done"}, nil
}

func TestParsedToolsStartImmediatelyAndReturnOneCompleteBatch(test *testing.T) {
	testStack := newStack(test)
	session := testStack.instance(test, 2)
	testStack.user(test, session, "run both tools")
	modelProvider := &streamedBatchProvider{
		fastStarted: make(chan struct{}), slowStarted: make(chan struct{}),
		releaseStream: make(chan struct{}), requests: make(chan atom.Request, 4),
	}
	testStack.harnessRuntime.Provider(modelProvider)
	session.Model = "batch-model"
	operationContext, cancelOperation := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelOperation()
	releaseSlowTool := make(chan struct{})
	modelResponseSaved := make(chan struct{}, 1)
	harness.Pipe(testStack.harnessRuntime, atom.StageModelResponse, func(operationContext context.Context, message atom.Message) (atom.Message, error) {
		if len(message.ToolCalls) == 2 {
			modelResponseSaved <- struct{}{}
		}
		return message, nil
	})
	testutil.RequireNoError(test, testStack.registry.Add(&fakeTool{name: "fast", run: func(context.Context, atom.ToolCall) (atom.ToolResult, error) {
		close(modelProvider.fastStarted)
		modelProvider.completedTools.Add(1)
		return atom.ToolResult{Status: atom.StatusError, Error: "fast tool failed"}, nil
	}}))
	testutil.RequireNoError(test, testStack.registry.Add(&fakeTool{name: "slow", run: func(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error) {
		close(modelProvider.slowStarted)
		select {
		case <-releaseSlowTool:
			modelProvider.completedTools.Add(1)
			return atom.ToolResult{Status: atom.StatusOK, Content: []atom.Content{{Type: atom.Text, Text: "slow result"}}}, nil
		case <-operationContext.Done():
			return atom.ToolResult{}, operationContext.Err()
		}
	}}))
	runFinished := make(chan error, 1)
	go func() { runFinished <- testStack.loop.Run(operationContext, session) }()
	select {
	case <-modelProvider.fastStarted:
	case <-operationContext.Done():
		test.Fatal("first parsed tool did not start before model EOF")
	}
	select {
	case <-modelProvider.slowStarted:
	case <-operationContext.Done():
		test.Fatal("second parsed tool did not start before model EOF")
	}
	if modelProvider.modelCalls.Load() != 1 {
		test.Fatal("a completed tool triggered a model request while the stream was open")
	}
	close(modelProvider.releaseStream)
	select {
	case <-modelResponseSaved:
	case <-operationContext.Done():
		test.Fatal("model response was not saved")
	}
	if modelProvider.modelCalls.Load() != 1 {
		test.Fatal("model continued before the slow tool finished")
	}
	close(releaseSlowTool)
	select {
	case operationError := <-runFinished:
		testutil.RequireNoError(test, operationError)
	case <-operationContext.Done():
		test.Fatal("tool batch did not finish")
	}
	if modelProvider.modelCalls.Load() != 2 {
		test.Fatalf("expected one continuation request, got %d total calls", modelProvider.modelCalls.Load())
	}
	<-modelProvider.requests
	request := <-modelProvider.requests
	if len(request.Messages) != 4 || request.Messages[1].Role != atom.RoleAssistant || len(request.Messages[1].ToolCalls) != 2 {
		test.Fatalf("unexpected continuation history: %+v", request.Messages)
	}
	for index, identifier := range []string{"slow-call", "fast-call"} {
		message := request.Messages[index+2]
		if message.Role != atom.RoleTool || message.ToolCallID != identifier {
			test.Fatalf("batch result %d: %+v", index, message)
		}
	}
	if request.Messages[2].Content[0].Text != "slow result" || request.Messages[3].Content[0].Text != "fast tool failed" {
		test.Fatalf("missing success or failure result in batch: %+v", request.Messages)
	}
}

func TestRuntimeQueueInputKeepsItsOrigin(test *testing.T) {
	testStack := newStack(test)
	session := testStack.instance(test, 2)
	messageQueue := newTestQueue(test, testStack.loop)
	turnFinished := make(chan struct{}, 1)
	testStack.harnessRuntime.On(atom.EventTurnEnd, func(context.Context, atom.Event) { turnFinished <- struct{}{} })
	message, _, operationError := messageQueue.SubmitRuntimeContent(context.Background(), session, []atom.Content{{Type: atom.Text, Text: "[process] background result"}})
	testutil.RequireNoError(test, operationError)
	if message.Role != atom.RoleRuntime {
		test.Fatalf("runtime submission was attributed to %q", message.Role)
	}
	select {
	case <-turnFinished:
	case <-time.After(2 * time.Second):
		test.Fatal("runtime input did not reach the loop")
	}
	testutil.RequireNoError(test, messageQueue.Close(context.Background()))
	messages, operationError := testStack.database.Sessions().Messages(context.Background(), session.ID)
	testutil.RequireNoError(test, operationError)
	if len(messages) != 2 || messages[0].Role != atom.RoleRuntime || messages[1].Role != atom.RoleAssistant {
		test.Fatalf("runtime input changed role in durable queue: %+v", messages)
	}
	if len(testStack.provider.Requests) != 1 || testStack.provider.Requests[0].Messages[0].Role != atom.RoleRuntime {
		test.Fatal("runtime origin was lost before provider dispatch")
	}
}
