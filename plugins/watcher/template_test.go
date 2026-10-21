package watcher

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func TestVariablesAreSinglePassAndWindowsAreValidated(test *testing.T) {
	text, operationError := renderVariables("{{todo}} {todo} {last_32k_context}", func(name string) (string, error) {
		switch name {
		case "todo":
			return "literal {doing}", nil
		case "last_32k_context":
			return "tail", nil
		}
		return "", fmt.Errorf("unexpected %s", name)
	})
	if operationError != nil || text != "{todo} literal {doing} tail" {
		test.Fatalf("render %q: %v", text, operationError)
	}
	for _, name := range []string{"{last_0_context}", "{last_1025k_context}", "{unknown}"} {
		if validateTemplate(name) == nil {
			test.Fatalf("accepted %s", name)
		}
	}
	for _, name := range []string{"{last_32k_context}", "{last_65536_context}", "{todo}", "{{unknown}}"} {
		if operationError := validateTemplate(name); operationError != nil {
			test.Fatal(operationError)
		}
	}
}
func TestTranscriptWindowKeepsLatestRowsAndMarksLargeUTF8Excerpt(test *testing.T) {
	measure := func(text string) (int, error) { return len(text), nil }
	text, omitted, operationError := selectTranscript([]string{"User:\n" + strings.Repeat("old ", 40), "Assistant:\nnew"}, 80, measure)
	if operationError != nil || !omitted || strings.Contains(text, "old") || !strings.Contains(text, "new") {
		test.Fatalf("window %q %v %v", text, omitted, operationError)
	}
	text, omitted, operationError = selectTranscript([]string{strings.Repeat("🌍", 200)}, 120, measure)
	if operationError != nil || !omitted || len(text) > 120 || !utf8.ValidString(text) || !strings.Contains(text, "message excerpt") {
		test.Fatalf("excerpt %q %v %v", text, omitted, operationError)
	}
}
func TestSchedulesUseRepeatedContextIntervalsAndRebaseOnRemoval(test *testing.T) {
	value := definition{ID: "one", Enabled: true, Triggers: []string{"context_growth", "end_turn"}, IntervalTokens: 256, MaxContinuations: 3}
	state := sessionState{Schedules: map[string]schedule{}}
	source := harness.ReviewSource{Fence: harness.ReviewFence{UserMessageID: "user"}}
	configuration := configuration{Enabled: true, Watchers: []definition{value}}
	observe := func(tokens int, visible []string) int {
		return len(selectDefinitions(&state, configuration, source, observation{Trigger: "context_growth", Tokens: tokens, Visible: visible, Model: "test/model"}))
	}
	if observe(255, []string{"old"}) != 0 || observe(256, []string{"old"}) != 1 || observe(300, []string{"old"}) != 0 || observe(512, []string{"old", "new"}) != 1 {
		test.Fatal("incorrect context cadence")
	}
	if observe(100, []string{"new"}) != 0 || observe(355, []string{"new"}) != 0 || observe(356, []string{"new"}) != 1 {
		test.Fatal("compaction did not rebase the next interval")
	}
	progress := state.Schedules[value.ID]
	progress.Continuations = 3
	state.Schedules[value.ID] = progress
	if len(selectDefinitions(&state, configuration, source, observation{Trigger: "end_turn"})) != 0 {
		test.Fatal("continuation limit reset at a turn boundary")
	}
	source.Fence.UserMessageID = "new-user"
	if len(selectDefinitions(&state, configuration, source, observation{Trigger: "end_turn"})) != 1 {
		test.Fatal("new user task did not reset continuation chain")
	}
}
func TestOutsidePermissionFilterUsesResolvedTargets(test *testing.T) {
	root := test.TempDir()
	source := harness.ReviewSource{Workspace: atom.InstanceSpec{Workspace: root}}
	value := definition{OutsideWorkspaceOnly: true, PermissionTools: []string{"file_actions"}}
	request := &atom.PermissionRequest{Target: "file_actions:" + root + "/inside", ToolCall: &atom.ToolCall{Name: "file_actions"}}
	if permissionMatches(value, source, request) {
		test.Fatal("inside workspace matched outside filter")
	}
	request.Target = "file_actions:" + test.TempDir() + "/outside"
	if !permissionMatches(value, source, request) {
		test.Fatal("outside target did not match")
	}
}

func TestPartialTranscriptIsExplicitlyInProgress(test *testing.T) {
	source := harness.ReviewSource{LiveMessageID: "live", Messages: []atom.Message{{ID: "saved", Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "task"}}}, {ID: "live", Role: atom.RoleAssistant, Content: []atom.Content{{Type: atom.Text, Text: "partial output"}}}}}
	rows := transcriptMessages(source, false)
	if len(rows) != 2 || strings.Contains(rows[0], "in progress") || !strings.Contains(rows[1], "Assistant (in progress):") {
		test.Fatalf("partial response lost its state: %v", rows)
	}
}

func TestTranscriptUsesVisibleTextAndPreciseToolArguments(testingHandle *testing.T) {
	source := harness.ReviewSource{Messages: []atom.Message{
		{ID: "private-user-id", SessionID: "private-session", Seq: 12345, Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "Check **the project**.\nKeep 🌍."}, {Type: atom.Image, Filename: "screen.png", MIME: "image/png", Data: []byte("private-media"), URL: "https://private.invalid/image"}}},
		{ID: "private-assistant-id", Role: atom.RoleAssistant, Reasoning: "supplied thinking", ToolCalls: []atom.ToolCall{{ID: "opaque-call-id", Name: "bash", Input: json.RawMessage(`{"command":"printf 'hello\\n'","timeout":9007199254740993}`)}}},
		{Role: atom.RoleTool, ToolCallID: "opaque-call-id", Content: []atom.Content{{Type: atom.Text, Text: `{"literal":"file data"}`}}},
	}}
	text := strings.Join(transcriptMessages(source, false), "\n\n")
	for _, expected := range []string{"User:", "Check **the project**.\nKeep 🌍.", "screen.png", "media content not supplied", "Tool call: bash", "command: printf 'hello\\n'", "timeout: 9007199254740993", "Tool result — bash:", `{"literal":"file data"}`} {
		if !strings.Contains(text, expected) {
			testingHandle.Fatalf("missing visible text %q: %s", expected, text)
		}
	}
	for _, forbidden := range []string{"private-user-id", "private-session", "opaque-call-id", "private-media", "private.invalid", "supplied thinking", `"CreatedAt"`, `"ToolCalls"`, `"Content"`} {
		if strings.Contains(text, forbidden) {
			testingHandle.Fatalf("serialized metadata escaped into transcript: %s", forbidden)
		}
	}
	if !strings.Contains(strings.Join(transcriptMessages(source, true), "\n"), "Supplied thinking:\nsupplied thinking") {
		testingHandle.Fatal("configured reasoning reference was removed")
	}
	encoded, _ := json.Marshal(source.Messages)
	if len(text) >= len(encoded)/2 {
		testingHandle.Fatalf("message envelope overhead remains: text=%d json=%d", len(text), len(encoded))
	}
}

func TestRuntimeWatcherEnvelopeBecomesVisibleFeedbackOnly(testingHandle *testing.T) {
	text := `[watcher]
{"type":"watcher.intervention","run_ids":["opaque"],"continue_chat":false,"notes":[{"name":"Review","message":"Which environment?"}]}`
	source := harness.ReviewSource{Messages: []atom.Message{{Role: atom.RoleRuntime, Content: []atom.Content{{Type: atom.Text, Text: text}}}}}
	rendered := strings.Join(transcriptMessages(source, false), "\n")
	if !strings.Contains(rendered, "Watcher question for the user:\nReview:\nWhich environment?") || strings.Contains(rendered, "run_ids") {
		testingHandle.Fatalf("runtime metadata was not projected: %s", rendered)
	}
	source.Messages[0].Role = atom.RoleUser
	if !strings.Contains(strings.Join(transcriptMessages(source, false), "\n"), text) {
		testingHandle.Fatal("literal user text was rewritten as a runtime envelope")
	}
}
