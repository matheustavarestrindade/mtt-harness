package provider

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func encodeResponsesAPIInput(messages []atom.Message, providerName string) ([]any, string, error) {
	input := make([]any, 0, len(messages))
	var instructions []string
	for _, message := range messages {
		if message.Role == atom.RoleSystem && !message.InContext {
			instructions = append(instructions, concatenateContentText(message.Content))
			continue
		}
		if message.Role == atom.RoleAssistant && message.ProviderState != nil && message.ProviderState.Provider == providerName {
			for _, raw := range message.ProviderState.Reasoning {
				var item map[string]any
				if operationError := json.Unmarshal(raw, &item); operationError != nil {
					return nil, "", fmt.Errorf("decode reasoning continuation: %w", operationError)
				}
				if item["type"] != "reasoning" {
					return nil, "", fmt.Errorf("provider continuation must contain reasoning items only")
				}
				input = append(input, item)
			}
		}
		if message.Role == atom.RoleTool {
			output, operationError := encodeResponsesAPIContent(message.Content)
			if operationError != nil {
				return nil, "", operationError
			}
			input = append(input, map[string]any{"type": "function_call_output", "call_id": message.ToolCallID, "output": output})
			continue
		}
		content, operationError := encodeResponsesAPIContent(message.Content)
		if operationError != nil {
			return nil, "", operationError
		}
		// An assistant can consist only of function calls; do not add an empty
		// assistant message between its reasoning and those calls.
		if message.Role != atom.RoleAssistant || concatenateContentText(message.Content) != "" || len(message.ToolCalls) == 0 {
			role := message.Role
			// Runtime data is a contextual input, never provider instructions or
			// an extra function_call_output for an already completed invocation.
			if role == atom.RoleRuntime {
				role = atom.RoleUser
			}
			input = append(input, map[string]any{"role": string(role), "content": content})
		}
		for _, call := range message.ToolCalls {
			arguments, operationError := call.ArgumentsText()
			if operationError != nil {
				return nil, "", operationError
			}
			input = append(input, map[string]any{"type": "function_call", "call_id": call.ID, "name": call.Name, "arguments": arguments})
		}
	}
	return input, strings.Join(instructions, "\n\n"), nil
}

func encodeResponsesAPIContent(contents []atom.Content) (any, error) {
	var parts []map[string]any
	hasMedia := false
	for _, content := range contents {
		switch content.Type {
		case atom.Text:
			parts = append(parts, map[string]any{"type": "input_text", "text": content.Text})
		case atom.Image:
			hasMedia = true
			imageURL, operationError := encodeImageURL(content)
			if operationError != nil {
				return nil, operationError
			}
			parts = append(parts, map[string]any{"type": "input_image", "image_url": imageURL})
		case atom.File:
			hasMedia = true
			fileData, operationError := encodeFileData(content)
			if operationError != nil {
				return nil, operationError
			}
			filename := content.Filename
			if filename == "" {
				filename = "attachment.bin"
			}
			parts = append(parts, map[string]any{"type": "input_file", "filename": filename, "file_data": fileData})
		case atom.Video:
			hasMedia = true
			videoURL, operationError := encodeVideoURL(content)
			if operationError != nil {
				return nil, operationError
			}
			parts = append(parts, map[string]any{"type": "input_video", "video_url": videoURL})
		default:
			return nil, fmt.Errorf("Responses API does not support content type %q in this adapter", content.Type)
		}
	}
	if !hasMedia {
		return concatenateContentText(contents), nil
	}
	return parts, nil
}
