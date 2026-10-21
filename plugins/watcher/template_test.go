package watcher

import (
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
	text, omitted, operationError := selectTranscript([]string{`{"old":1}`, `{"new":2}`}, 15, measure)
	if operationError != nil || !omitted || strings.Contains(text, "old") || !strings.Contains(text, "new") {
		test.Fatalf("window %q %v %v", text, omitted, operationError)
	}
	text, omitted, operationError = selectTranscript([]string{strings.Repeat("🌍", 200)}, 120, measure)
	if operationError != nil || !omitted || len(text) > 120 || !utf8.ValidString(text) || !strings.Contains(text, "truncated") {
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
	if len(rows) != 2 || strings.Contains(rows[0], "InProgress") || !strings.Contains(rows[1], `"InProgress":true`) {
		test.Fatalf("partial response lost its state: %v", rows)
	}
}
