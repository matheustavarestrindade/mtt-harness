package provider

import (
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestRuntimeDataIsNotSystemInstructionsOrAnExtraToolReply(test *testing.T) {
	content := "[process] Runtime update from a background process, not a new user request.\noutput"
	messages := []atom.Message{
		{Role: atom.RoleSystem, Content: []atom.Content{{Type: atom.Text, Text: "system instructions"}}},
		{Role: atom.RoleAssistant, ToolCalls: []atom.ToolCall{{ID: "background-call", Name: "bash", Input: []byte(`{}`)}}},
		{Role: atom.RoleTool, ToolCallID: "background-call", Content: []atom.Content{{Type: atom.Text, Text: "process started"}}},
		{Role: atom.RoleRuntime, Content: []atom.Content{{Type: atom.Text, Text: content}}},
	}
	chat, operationError := encodeMessages(messages, "fixture")
	testutil.RequireNoError(test, operationError)
	if len(chat) != 4 || chat[2]["role"] != "tool" || chat[3]["role"] != "user" || chat[3]["content"] != content {
		test.Fatalf("runtime input has an invalid Chat Completions role or content: %+v", chat)
	}
	if _, duplicate := chat[3]["tool_call_id"]; duplicate {
		test.Fatal("runtime update duplicates an already completed tool reply")
	}
	responses, instructions, operationError := encodeResponsesAPIInput(messages, "fixture")
	testutil.RequireNoError(test, operationError)
	if instructions != "system instructions" || strings.Contains(instructions, "process") {
		test.Fatalf("runtime output became system instructions: %q", instructions)
	}
	if len(responses) != 3 {
		test.Fatalf("unexpected Responses input: %+v", responses)
	}
	update := responses[2].(map[string]any)
	if update["role"] != "user" || update["content"] != content || update["type"] == "function_call_output" {
		test.Fatalf("runtime update has an invalid Responses encoding: %+v", update)
	}
	if messages[3].Role != atom.RoleRuntime {
		test.Fatal("provider encoding changed the stored runtime origin")
	}
}
