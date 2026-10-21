package provider

import (
	"encoding/json"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func TestAdaptersReplayRejectedArgumentsAsOriginalText(testingHandle *testing.T) {
	raw := `{"path":"example.txt"`
	encoded, _ := json.Marshal(raw)
	messages := []atom.Message{{Role: atom.RoleAssistant, ToolCalls: []atom.ToolCall{{ID: "bad", Name: "inspect", Input: encoded, InputError: "invalid JSON"}}}}
	responses, _, operationError := encodeResponsesAPIInput(messages, "synthetic")
	if operationError != nil {
		testingHandle.Fatal(operationError)
	}
	if responses[0].(map[string]any)["arguments"] != raw {
		testingHandle.Fatal("Responses changed malformed source arguments")
	}
	chat, operationError := encodeMessages(messages, "synthetic")
	if operationError != nil {
		testingHandle.Fatal(operationError)
	}
	calls := chat[0]["tool_calls"].([]map[string]any)
	if calls[0]["function"].(map[string]any)["arguments"] != raw {
		testingHandle.Fatal("Chat Completions changed malformed source arguments")
	}
}
