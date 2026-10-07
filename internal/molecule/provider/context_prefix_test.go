package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestMutableRuntimeTailKeepsNativeProviderPrefix(test *testing.T) {
	base := []atom.Message{{Role: atom.RoleSystem, Ephemeral: true, Content: []atom.Content{{Type: atom.Text, Text: "Fixed startup policy."}}}, {Role: atom.RoleSystem, Ephemeral: true, Content: []atom.Content{{Type: atom.Text, Text: "<memory>\nL Use PostgreSQL.\n</memory>"}}}, {ID: "input", Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "[context_message id=12]\nDo the task."}}}}
	first := append(append([]atom.Message(nil), base...), atom.Message{Role: atom.RoleRuntime, Ephemeral: true, Content: []atom.Content{{Type: atom.Text, Text: "task revision 1; context input 50%"}}})
	second := append(append([]atom.Message(nil), base...), atom.Message{Role: atom.RoleRuntime, Ephemeral: true, Content: []atom.Content{{Type: atom.Text, Text: "task revision 2; context input 65%"}}})
	firstInput, firstInstructions, operationError := encodeResponsesAPIInput(first, "fixture")
	testutil.RequireNoError(test, operationError)
	secondInput, secondInstructions, operationError := encodeResponsesAPIInput(second, "fixture")
	testutil.RequireNoError(test, operationError)
	if firstInstructions != secondInstructions || strings.Contains(firstInstructions, "revision") {
		test.Fatal("Responses system instructions changed with runtime data")
	}
	firstPrefix, _ := json.Marshal(firstInput[:len(firstInput)-1])
	secondPrefix, _ := json.Marshal(secondInput[:len(secondInput)-1])
	if string(firstPrefix) != string(secondPrefix) {
		test.Fatal("Responses conversation prefix changed")
	}
	firstChat, operationError := encodeMessages(first, "fixture")
	testutil.RequireNoError(test, operationError)
	secondChat, operationError := encodeMessages(second, "fixture")
	testutil.RequireNoError(test, operationError)
	firstPrefix, _ = json.Marshal(firstChat[:len(firstChat)-1])
	secondPrefix, _ = json.Marshal(secondChat[:len(secondChat)-1])
	if string(firstPrefix) != string(secondPrefix) {
		test.Fatal("Chat Completions prefix changed")
	}
}
