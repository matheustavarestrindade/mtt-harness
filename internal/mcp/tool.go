package mcp

import (
	"context"
	"encoding/base64"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type Tool struct {
	Client *Client
	Server string
	Spec   ToolSpec
}

func (tool *Tool) Name() string {
	return "mcp__" + tool.Server + "__" + tool.Spec.Name
}

func (tool *Tool) Description() string {
	if tool.Spec.Description == "" {
		return "The MCP tool " + tool.Spec.Name + " of the server " + tool.Server + "."
	}
	return tool.Spec.Description
}

func (tool *Tool) Categories() []string {
	return []string{"mcp", tool.Server}
}

func (tool *Tool) InputSchema() atom.Schema {
	if len(tool.Spec.InputSchema) == 0 {
		return atom.Schema{JSON: []byte(`{"type":"object"}`)}
	}
	return atom.Schema{JSON: tool.Spec.InputSchema}
}

func (tool *Tool) Check(operationContext context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

func (tool *Tool) Run(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	result, operationError := tool.Client.CallTool(operationContext, tool.Spec.Name, call.Input)
	if operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: operationError.Error()}, nil
	}
	toolResult := atom.ToolResult{CallID: call.ID, Status: atom.StatusOK}
	for _, item := range result.Content {
		switch item.Type {
		case "image", "audio":
			data, operationError := base64.StdEncoding.DecodeString(item.Data)
			if operationError != nil {
				continue
			}
			media := atom.Image
			if item.Type == "audio" {
				media = atom.Audio
			}
			toolResult.Content = append(toolResult.Content, atom.Content{Type: media, Data: data, MIME: item.MimeType})
		default:
			if item.Text != "" {
				toolResult.Content = append(toolResult.Content, atom.Content{Type: atom.Text, Text: item.Text})
			}
		}
	}
	if result.IsError {
		toolResult.Status = atom.StatusError
		if len(toolResult.Content) > 0 {
			toolResult.Error = toolResult.Content[0].Text
		}
	}
	return toolResult, nil
}
