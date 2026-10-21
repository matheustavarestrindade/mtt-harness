package sidekick

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestSidekickLanguageUsesBoundedHumanReferences(testingHandle *testing.T) {
	message := func(id string, role atom.Role, text string) atom.Message {
		return atom.Message{ID: id, Role: role, Content: []atom.Content{{Type: atom.Text, Text: text}}}
	}
	user := message("latest", atom.RoleUser, "yes, continue")
	messages := []atom.Message{message("earlier", atom.RoleUser, "Please check the migration before making changes."), message("assistant", atom.RoleAssistant, "请使用中文回答。"), message("runtime", atom.RoleRuntime, "来自以前的助手备注。"), message("tool", atom.RoleTool, "这些是检索结果。"), user, message("future", atom.RoleUser, "A later request must not change this job's language reference.")}
	references := buildUserLanguageReference(user, messages)
	if len(references) != 2 || references[0] != "yes, continue" || references[1] != messageText(messages[0]) {
		testingHandle.Fatalf("language reference mixed generated/future content: %q", references)
	}
	user.Content[0].Text = strings.Repeat("é🌍", 2000)
	references = buildUserLanguageReference(user, messages)
	if len(references) > 3 || len(references[0]) > 512 || !utf8.ValidString(references[0]) {
		testingHandle.Fatalf("unbounded or invalid language reference: %d", len(references[0]))
	}
	if len(buildUserLanguageReference(message("not-human", atom.RoleRuntime, "generated"), messages)) != 0 {
		testingHandle.Fatal("runtime input became a human language cue")
	}
}

func TestSidekickPassesLanguageReferenceToTheWorker(testingHandle *testing.T) {
	fixture := newFixture(testingHandle, false)
	user := fixture.services.messages[0]
	fixture.services.messages = append(fixture.services.messages, atom.Message{ID: "generated", Role: atom.RoleAssistant, Content: []atom.Content{{Type: atom.Text, Text: "之前的提示使用中文。"}}})
	fixture.services.run = func(_ context.Context, request harness.WorkspaceAgentRequest) (harness.WorkspaceAgentResponse, error) {
		payload := messageText(request.Messages[1])
		_, encoded, found := strings.Cut(payload, "\n")
		if !found {
			testingHandle.Fatal("missing quoted worker input")
		}
		var input struct {
			User     string   `json:"user_request"`
			Language []string `json:"user_language_reference"`
			Recent   []any    `json:"recent_context_excerpt"`
		}
		testutil.RequireNoError(testingHandle, json.Unmarshal([]byte(encoded), &input))
		if input.User != messageText(user) || len(input.Language) != 1 || input.Language[0] != messageText(user) || len(input.Recent) != 2 {
			testingHandle.Fatalf("worker language contract changed: %+v", input)
		}
		return harness.WorkspaceAgentResponse{Text: `{"notes":[{"text":"The schema keeps workspace ownership explicit.","sources":["file:one"]}]}`}, nil
	}
	configuration := defaultConfiguration()
	configuration.WorkerModel = "fixture/worker"
	text, used, operationError := fixture.plugin.filterSources(context.Background(), fixture.session, fixture.services.task, user, configuration, []sourceData{{Key: "file:one", Origin: "schema.go:1-4", Text: "Workspace ownership is explicit in the schema."}}, "language-test")
	testutil.RequireNoError(testingHandle, operationError)
	if len(used) != 1 || !strings.Contains(text, "schema.go:1-4") {
		testingHandle.Fatalf("language projection lost source evidence: %s", text)
	}
}
