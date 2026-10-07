package contextplugin

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestRememberSavesExactTextBeforeReturningWithoutModelWork(test *testing.T) {
	fixture := newPluginFixture(test)
	note := "  My name is Matheus.\nUse café, not cafe.  "
	input, operationError := json.Marshal(rememberInput{Text: note, Categories: []string{"preferences"}})
	testutil.RequireNoError(test, operationError)
	tool := memoryTool{plugin: fixture.plugin, name: "remember"}
	operationContext := harness.WithSession(context.Background(), fixture.session)
	result, operationError := tool.Run(operationContext, atom.ToolCall{ID: "direct-note", Name: "remember", Input: input})
	testutil.RequireNoError(test, operationError)
	if result.Content[0].Text != `{"state":"saved"}` {
		test.Fatalf("unexpected save receipt: %s", result.Content[0].Text)
	}
	for _, compression := range []string{"low", "medium", "high"} {
		reply, operationError := fixture.plugin.searchMemory(operationContext, fixture.session, memorySearchInput{Query: "Matheus", Compression: compression})
		testutil.RequireNoError(test, operationError)
		if len(reply.Matches) != 1 || reply.Matches[0].Data != note {
			test.Fatalf("%s changed the saved note: %+v", compression, reply.Matches)
		}
	}
	_, operationError = tool.Run(operationContext, atom.ToolCall{ID: "direct-note", Name: "remember", Input: input})
	testutil.RequireNoError(test, operationError)
	counts, operationError := fixture.plugin.database.Count(operationContext, fixture.session.InstanceID)
	testutil.RequireNoError(test, operationError)
	if counts["memory/active"] != 1 {
		test.Fatalf("repeated call created another note: %+v", counts)
	}
	jobs, operationError := fixture.plugin.database.List(operationContext, fixture.session.InstanceID, "job", "", 20)
	testutil.RequireNoError(test, operationError)
	if len(jobs) != 0 {
		test.Fatal("remember queued a background validation job")
	}
	fixture.services.mutex.Lock()
	defer fixture.services.mutex.Unlock()
	if len(fixture.services.calls) != 0 {
		test.Fatal("remember invoked a language model")
	}
}

func TestRememberMissingReplacementDoesNotWriteAMemory(test *testing.T) {
	fixture := newPluginFixture(test)
	oldText := "A note that does not exist"
	_, operationError := fixture.plugin.saveMemory(context.Background(), fixture.session, "failed-replacement", rememberInput{Text: "New note", OldText: &oldText})
	if operationError == nil {
		test.Fatal("missing exact replacement was accepted")
	}
	counts, operationError := fixture.plugin.database.Count(context.Background(), fixture.session.InstanceID)
	testutil.RequireNoError(test, operationError)
	if counts["memory/active"] != 0 || counts["remember/"] != 0 {
		test.Fatalf("failed replacement persisted a note: %+v", counts)
	}
}

func TestSelectionReferencesStayOutsideConversationText(test *testing.T) {
	request := atom.Request{Messages: []atom.Message{
		{ID: "user", Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "My name is Matheus."}}},
		{ID: "answer", Role: atom.RoleAssistant, Content: []atom.Content{{Type: atom.Text, Text: "I'll call you Matheus."}}},
	}}
	before, _ := json.Marshal(request)
	view := newSessionView("fixture")
	view.Initialized = true
	view.Sources = map[string]int64{"user": 26, "answer": 27}
	projected := projectContext(request, view)
	for _, original := range request.Messages {
		found := false
		for _, message := range projected.Messages {
			if message.ID == original.ID {
				found = true
				if !reflect.DeepEqual(message, original) {
					test.Fatal("context references changed conversation text")
				}
			}
		}
		if !found {
			test.Fatal("projection removed a conversation message")
		}
	}
	tail := projected.Messages[len(projected.Messages)-1]
	if tail.Role != atom.RoleRuntime || !tail.Ephemeral {
		test.Fatal("selection references were not request-local tail data")
	}
	var data struct {
		Messages [][2]any `json:"messages"`
	}
	testutil.RequireNoError(test, json.Unmarshal([]byte(tail.Content[0].Text[len("Context selection data: "):]), &data))
	if len(data.Messages) != 2 || data.Messages[1][0] != float64(27) || data.Messages[1][1] != "assistant" {
		test.Fatal("selection IDs lost their message mapping")
	}
	after, _ := json.Marshal(request)
	if string(before) != string(after) {
		test.Fatal("projection mutated stored messages")
	}
}

func TestLegacyAssistantLabelIsNotReplayedAsAnAnswerExample(test *testing.T) {
	message := atom.Message{Role: atom.RoleAssistant, Content: []atom.Content{{Type: atom.Text, Text: "[context_message id=27 role=assistant]\n\nHello, Matheus."}}}
	projected := withoutEchoedContextLabel(message)
	if projected.Content[0].Text != "Hello, Matheus." || message.Content[0].Text == projected.Content[0].Text {
		test.Fatal("legacy label cleanup changed persisted text or retained the label")
	}
	message.Role = atom.RoleUser
	if !reflect.DeepEqual(withoutEchoedContextLabel(message), message) {
		test.Fatal("user text was changed")
	}
}
