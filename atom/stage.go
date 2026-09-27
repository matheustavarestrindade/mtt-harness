package atom

type Stage[T any] struct {
	Name string
}

func NewStage[T any](name string) Stage[T] {
	return Stage[T]{Name: name}
}

var (
	StageContextBuild  = NewStage[[]Message]("context.build")
	StageModelRequest  = NewStage[Request]("model.request")
	StageModelResponse = NewStage[Message]("model.response")
	StageActionPlan    = NewStage[ToolCall]("action.plan")
	StageToolInput     = NewStage[ToolCall]("tool.input")
	StageToolResult    = NewStage[ToolResult]("tool.result")
	StageMessageSave   = NewStage[Message]("message.save")
	StageProcessOutput = NewStage[ProcessEvent]("process.output")
)
