package pipeline

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type Pipeline struct {
	h *harness.Harness
}

func New(h *harness.Harness) *Pipeline {
	return &Pipeline{h: h}
}

func (p *Pipeline) Context(ctx context.Context, messages []atom.Message) ([]atom.Message, error) {
	return harness.Run(ctx, p.h, atom.StageContextBuild, messages)
}

func (p *Pipeline) Request(ctx context.Context, request atom.Request) (atom.Request, error) {
	return harness.Run(ctx, p.h, atom.StageModelRequest, request)
}

func (p *Pipeline) Response(ctx context.Context, message atom.Message) (atom.Message, error) {
	return harness.Run(ctx, p.h, atom.StageModelResponse, message)
}

func (p *Pipeline) Plan(ctx context.Context, call atom.ToolCall) (atom.ToolCall, error) {
	return harness.Run(ctx, p.h, atom.StageActionPlan, call)
}

func (p *Pipeline) Input(ctx context.Context, call atom.ToolCall) (atom.ToolCall, error) {
	return harness.Run(ctx, p.h, atom.StageToolInput, call)
}

func (p *Pipeline) Result(ctx context.Context, result atom.ToolResult) (atom.ToolResult, error) {
	return harness.Run(ctx, p.h, atom.StageToolResult, result)
}

func (p *Pipeline) Save(ctx context.Context, message atom.Message) (atom.Message, error) {
	return harness.Run(ctx, p.h, atom.StageMessageSave, message)
}

func (p *Pipeline) Output(ctx context.Context, event atom.ProcessEvent) (atom.ProcessEvent, error) {
	return harness.Run(ctx, p.h, atom.StageProcessOutput, event)
}

func (p *Pipeline) DecideInput(ctx context.Context, call atom.ToolCall) (atom.Verdict, error) {
	return harness.Check(ctx, p.h, atom.StageToolInput, call)
}

func (p *Pipeline) DecideRequest(ctx context.Context, request atom.Request) (atom.Verdict, error) {
	return harness.Check(ctx, p.h, atom.StageModelRequest, request)
}
