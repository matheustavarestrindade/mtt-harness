package atom

import (
	"encoding/json"
	"fmt"
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

// ToolReference is a compact discovery result. The loop obtains callable
// instructions and schemas from the authoritative registry, not this result.
type ToolReference struct {
	Name       string
	Categories []string
}

type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
	// InputError rejects execution before tools, middleware or permissions run.
	// When set, Input is a JSON string containing the exact malformed argument
	// text. This keeps storage valid JSON without inventing replacement input.
	InputError string `json:",omitempty"`
}

// ArgumentsText returns the provider-visible argument text, including rejected
// input. Callers must never execute a call whose InputError is non-empty.
func (call ToolCall) ArgumentsText() (string, error) {
	if call.InputError == "" {
		return string(call.Input), nil
	}
	var arguments string
	if operationError := json.Unmarshal(call.Input, &arguments); operationError != nil {
		return "", fmt.Errorf("decode rejected tool arguments: %w", operationError)
	}
	return arguments, nil
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
