package pipeline

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type Pipeline struct {
	harnessRuntime *harness.Harness
}

func New(harnessRuntime *harness.Harness) *Pipeline {
	return &Pipeline{harnessRuntime: harnessRuntime}
}

func (pipeline *Pipeline) Context(operationContext context.Context, messages []atom.Message) ([]atom.Message, error) {
	return harness.Run(operationContext, pipeline.harnessRuntime, atom.StageContextBuild, messages)
}

func (pipeline *Pipeline) Request(operationContext context.Context, request atom.Request) (atom.Request, error) {
	return harness.Run(operationContext, pipeline.harnessRuntime, atom.StageModelRequest, request)
}

func (pipeline *Pipeline) Response(operationContext context.Context, message atom.Message) (atom.Message, error) {
	return harness.Run(operationContext, pipeline.harnessRuntime, atom.StageModelResponse, message)
}

func (pipeline *Pipeline) Plan(operationContext context.Context, call atom.ToolCall) (atom.ToolCall, error) {
	return harness.Run(operationContext, pipeline.harnessRuntime, atom.StageActionPlan, call)
}

func (pipeline *Pipeline) Input(operationContext context.Context, call atom.ToolCall) (atom.ToolCall, error) {
	return harness.Run(operationContext, pipeline.harnessRuntime, atom.StageToolInput, call)
}

func (pipeline *Pipeline) Result(operationContext context.Context, result atom.ToolResult) (atom.ToolResult, error) {
	return harness.Run(operationContext, pipeline.harnessRuntime, atom.StageToolResult, result)
}

func (pipeline *Pipeline) Save(operationContext context.Context, message atom.Message) (atom.Message, error) {
	return harness.Run(operationContext, pipeline.harnessRuntime, atom.StageMessageSave, message)
}

func (pipeline *Pipeline) Output(operationContext context.Context, event atom.ProcessEvent) (atom.ProcessEvent, error) {
	return harness.Run(operationContext, pipeline.harnessRuntime, atom.StageProcessOutput, event)
}

func (pipeline *Pipeline) DecideInput(operationContext context.Context, call atom.ToolCall) (atom.Verdict, error) {
	return harness.Check(operationContext, pipeline.harnessRuntime, atom.StageToolInput, call)
}

func (pipeline *Pipeline) DecideRequest(operationContext context.Context, request atom.Request) (atom.Verdict, error) {
	return harness.Check(operationContext, pipeline.harnessRuntime, atom.StageModelRequest, request)
}
