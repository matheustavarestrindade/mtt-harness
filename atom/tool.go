package atom

import (
	"encoding/json"
	"time"
)

type Schema struct {
	JSON json.RawMessage
}

type ToolSpec struct {
	Name        string
	Description string
	Categories  []string
	InputSchema Schema
}

type ToolGroup []ToolSpec

type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

type Status string

const (
	StatusOK     Status = "ok"
	StatusDenied Status = "denied"
	StatusError  Status = "error"
)

type ToolResult struct {
	CallID   string
	Status   Status
	Content  []Content
	Error    string
	Duration time.Duration
}

func (toolResult ToolResult) Text() string {
	var builder []byte
	for _, item := range toolResult.Content {
		if item.Type != Text {
			continue
		}
		if len(builder) > 0 {
			builder = append(builder, '\n')
		}
		builder = append(builder, item.Text...)
	}
	if toolResult.Error != "" {
		if len(builder) > 0 {
			builder = append(builder, '\n')
		}
		builder = append(builder, toolResult.Error...)
	}
	return string(builder)
}
