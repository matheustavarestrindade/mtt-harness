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

func (t *Tool) Name() string {
	return "mcp__" + t.Server + "__" + t.Spec.Name
}

func (t *Tool) Description() string {
	if t.Spec.Description == "" {
		return "The MCP tool " + t.Spec.Name + " of the server " + t.Server + "."
	}
	return t.Spec.Description
}

func (t *Tool) Categories() []string {
	return []string{"mcp", t.Server}
}

func (t *Tool) InputSchema() atom.Schema {
	if len(t.Spec.InputSchema) == 0 {
		return atom.Schema{JSON: []byte(`{"type":"object"}`)}
	}
	return atom.Schema{JSON: t.Spec.InputSchema}
}

func (t *Tool) Check(ctx context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

func (t *Tool) Run(ctx context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	result, err := t.Client.CallTool(ctx, t.Spec.Name, call.Input)
	if err != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: err.Error()}, nil
	}
	toolResult := atom.ToolResult{CallID: call.ID, Status: atom.StatusOK}
	for _, item := range result.Content {
		switch item.Type {
		case "image", "audio":
			data, err := base64.StdEncoding.DecodeString(item.Data)
			if err != nil {
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
