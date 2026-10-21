package watcher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

// Keep the same visible facts as the conversation view without serializing
// message envelopes, internal IDs, timestamps, usage, or private provider state.
func transcriptMessages(source harness.ReviewSource, includeReasoning bool) []string {
	toolNames := map[string]string{}
	for _, message := range source.Messages {
		for _, call := range message.ToolCalls {
			toolNames[call.ID] = call.Name
		}
	}
	var rows []string
	for _, message := range source.Messages {
		role := map[atom.Role]string{atom.RoleUser: "User", atom.RoleAssistant: "Assistant", atom.RoleTool: "Tool result", atom.RoleRuntime: "Runtime data", atom.RoleSystem: "System reference"}[message.Role]
		if role == "" {
			role = "Conversation entry"
		}
		if message.Role == atom.RoleTool {
			if name := toolNames[message.ToolCallID]; name != "" {
				role += " — " + name
			}
		}
		if source.LiveMessageID != "" && message.ID == source.LiveMessageID {
			role += " (in progress)"
		}
		var output strings.Builder
		output.WriteString(role + ":\n")
		for _, content := range message.Content {
			if content.Type == atom.Text {
				text := content.Text
				if message.Role == atom.RoleRuntime {
					text = runtimeReferenceText(text)
				}
				output.WriteString(text)
				output.WriteByte('\n')
				continue
			}
			output.WriteString("[" + string(content.Type))
			if content.Filename != "" {
				output.WriteString(": " + content.Filename)
			}
			if content.MIME != "" {
				output.WriteString(" · " + content.MIME)
			}
			output.WriteString("; media content not supplied]\n")
		}
		if includeReasoning && message.Reasoning != "" {
			output.WriteString("\nSupplied thinking:\n" + message.Reasoning + "\n")
		}
		for _, call := range message.ToolCalls {
			output.WriteString("\n" + formatToolCall(&call) + "\n")
		}
		rows = append(rows, strings.TrimSpace(output.String()))
	}
	return rows
}

func runtimeReferenceText(text string) string {
	if !strings.HasPrefix(text, "[watcher]\n") {
		return text
	}
	var envelope struct {
		Type         string `json:"type"`
		ContinueChat *bool  `json:"continue_chat"`
		Notes        []struct {
			Name    string `json:"name"`
			Message string `json:"message"`
		} `json:"notes"`
	}
	if json.Unmarshal([]byte(strings.TrimPrefix(text, "[watcher]\n")), &envelope) != nil || envelope.Type != "watcher.intervention" || len(envelope.Notes) == 0 {
		return text
	}
	var output strings.Builder
	if envelope.ContinueChat != nil && !*envelope.ContinueChat {
		output.WriteString("Watcher question for the user:\n")
	} else {
		output.WriteString("Watcher feedback:\n")
	}
	for _, note := range envelope.Notes {
		if note.Name != "" {
			output.WriteString(note.Name + ":\n")
		}
		output.WriteString(note.Message + "\n")
	}
	return strings.TrimSpace(output.String())
}

func formatToolCall(call *atom.ToolCall) string {
	if call == nil {
		return "No pending tool call."
	}
	var output strings.Builder
	output.WriteString("Tool call: " + call.Name + "\n")
	var arguments any
	decoder := json.NewDecoder(bytes.NewReader(call.Input))
	decoder.UseNumber()
	if operationError := decoder.Decode(&arguments); operationError != nil {
		return output.String() + "Arguments:\n" + string(call.Input)
	}
	writeReferenceValue(&output, arguments, "")
	return strings.TrimSpace(output.String())
}

func writeReferenceValue(output *strings.Builder, value any, indent string) {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if len(keys) == 0 {
			output.WriteString(indent + "(empty object)\n")
		}
		for _, key := range keys {
			output.WriteString(indent + key + ":")
			switch typed[key].(type) {
			case map[string]any, []any:
				output.WriteByte('\n')
				writeReferenceValue(output, typed[key], indent+"  ")
			default:
				output.WriteByte(' ')
				writeReferenceValue(output, typed[key], "")
			}
		}
	case []any:
		if len(typed) == 0 {
			output.WriteString(indent + "(empty list)\n")
		}
		for index, item := range typed {
			output.WriteString(fmt.Sprintf("%s%d. ", indent, index+1))
			switch item.(type) {
			case map[string]any, []any:
				output.WriteByte('\n')
				writeReferenceValue(output, item, indent+"  ")
			default:
				writeReferenceValue(output, item, "")
			}
		}
	case string:
		if typed == "" {
			output.WriteString(indent + "(empty string)\n")
		} else {
			output.WriteString(indent + typed + "\n")
		}
	case nil:
		output.WriteString(indent + "(not set)\n")
	default:
		output.WriteString(fmt.Sprintf("%s%v\n", indent, typed))
	}
}

func taskReferences(tasks atom.TaskState) (string, string) {
	var todo strings.Builder
	for _, item := range tasks.Todo {
		fmt.Fprintf(&todo, "- [%s] %s\n", item.Status, item.Title)
	}
	todoText := strings.TrimSpace(todo.String())
	if todoText == "" {
		todoText = "No TODO items."
	}
	doing := "No current DOING task."
	if tasks.Doing != nil {
		doing = tasks.Doing.Title
		if tasks.Doing.Description != "" {
			doing += "\n" + tasks.Doing.Description
		}
	}
	return todoText, doing
}

func permissionReference(request *atom.PermissionRequest) string {
	if request == nil {
		return "No pending permission request."
	}
	return "Target: " + request.Target + "\nReason: " + request.Why
}
