package atom

import (
	"encoding/json"
	"testing"
)

func TestRejectedToolInputRoundTrip(testingHandle *testing.T) {
	raw := "{\"path\":\"例.txt\",\n"
	encoded, operationError := json.Marshal(raw)
	if operationError != nil {
		testingHandle.Fatal(operationError)
	}
	call := ToolCall{ID: "call", Name: "read", Input: encoded, InputError: "invalid JSON"}
	stored, operationError := json.Marshal(call)
	if operationError != nil {
		testingHandle.Fatal(operationError)
	}
	var restored ToolCall
	if operationError = json.Unmarshal(stored, &restored); operationError != nil {
		testingHandle.Fatal(operationError)
	}
	text, operationError := restored.ArgumentsText()
	if operationError != nil || text != raw || restored.InputError != call.InputError {
		testingHandle.Fatalf("rejected arguments changed: %q %v", text, operationError)
	}
	if _, operationError = (ToolCall{Input: []byte(`{}`), InputError: "invalid"}).ArgumentsText(); operationError == nil {
		testingHandle.Fatal("corrupt error representation was accepted")
	}
}
