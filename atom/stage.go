package atom

type Stage[Value any] struct {
	Name string
}

func NewStage[Value any](name string) Stage[Value] {
	return Stage[Value]{Name: name}
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
