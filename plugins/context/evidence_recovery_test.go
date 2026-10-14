package contextplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestEvidenceFailureDescribesTheRejectedSourceWithoutRelaxingQuotes(test *testing.T) {
	sources := []source{{ID: 17, Role: atom.RoleUser, Text: "Keep **the exact** source.\nDo not change it."}}
	proposal := memoryProposal{Title: "Constraint", Categories: []string{"coding"}, Kind: "user_fact", Text: summaries{Low: "Keep the exact source.", Medium: "Keep the source.", High: "Keep source."}, Evidence: []memoryEvidence{{SourceID: 17, Quote: "Keep the exact source."}}}
	encode := func() string {
		encoded, _ := json.Marshal(map[string]any{"memories": []memoryProposal{proposal}})
		return string(encoded)
	}
	_, operationError := parseProposals(encode(), sources, nil)
	if operationError == nil || !strings.Contains(operationError.Error(), "source_id 17") || !strings.Contains(operationError.Error(), "not a literal excerpt") {
		test.Fatalf("validation hid the failing reference: %v", operationError)
	}
	proposal.Evidence[0].Quote = "Keep **the exact** source."
	_, operationError = parseProposals(encode(), sources, nil)
	testutil.RequireNoError(test, operationError)
	proposal.Evidence[0].SourceID = 999
	_, operationError = parseProposals(encode(), sources, nil)
	if operationError == nil || !strings.Contains(operationError.Error(), "source was not supplied") {
		test.Fatal("invented source was accepted")
	}
}

func TestExtractionRepairsRejectedPagesWithIndependentRetryBudgets(test *testing.T) {
	fixture := newPluginFixture(test)
	messages := fixtureMessages(fixture.session, 1000)
	fixture.prepare(test, messages, 200000)
	_, operationError := fixture.plugin.UpdateConfiguration(context.Background(), fixture.session.InstanceID, json.RawMessage(`{"retry_limit":1,"worker_count":1}`))
	testutil.RequireNoError(test, operationError)
	var mutex sync.Mutex
	attempts := map[int64]int{}
	validatedRepairs := 0
	fixture.services.mutex.Lock()
	fixture.services.respond = func(operationContext context.Context, input harness.WorkspaceAgentRequest) (harness.WorkspaceAgentResponse, error) {
		var page extractionInput
		if operationError := json.Unmarshal([]byte(input.Messages[1].Content[0].Text), &page); operationError != nil {
			return harness.WorkspaceAgentResponse{}, operationError
		}
		var selected sourceFragment
		for _, fragment := range page.Sources {
			if !fragment.ContextOnly {
				selected = fragment
				break
			}
		}
		if selected.Role == atom.RoleTool {
			return harness.WorkspaceAgentResponse{Text: `{"memories":[]}`}, nil
		}
		mutex.Lock()
		defer mutex.Unlock()
		attempts[selected.ID]++
		quote := "invented quote that is not in the source"
		if attempts[selected.ID] > 1 {
			if !strings.Contains(page.PreviousError, fmt.Sprintf("source_id %d", selected.ID)) || !strings.Contains(page.PreviousOutput, quote) {
				return harness.WorkspaceAgentResponse{}, fmt.Errorf("retry did not receive rejected output and precise evidence metadata")
			}
			quote = boundedUTF8(selected.Text, 100)
			validatedRepairs++
		}
		proposal := memoryProposal{Title: "Observed source", Categories: []string{"coding"}, Kind: "observation", Text: summaries{Low: "An observed source remains available.", Medium: "Source remains available.", High: "Source retained."}, Evidence: []memoryEvidence{{SourceID: selected.ID, Quote: quote}}}
		encoded, _ := json.Marshal(map[string]any{"memories": []memoryProposal{proposal}})
		return harness.WorkspaceAgentResponse{Model: input.Model, Text: string(encoded), Usage: atom.Usage{Input: 100, Output: 50}}, operationContext.Err()
	}
	fixture.services.mutex.Unlock()
	view := fixture.view(test)
	// The user source and the complete older tool group each reject once. A
	// global one-failure allowance exhausted before later pages could repair.
	dropMessages(test, fixture, []int64{view.Sources["decision"], view.Sources["tool-plan"], view.Sources["tool-result"]}, true)
	job := waitForJob(test, fixture, "ready")
	mutex.Lock()
	repairs := validatedRepairs
	mutex.Unlock()
	if repairs < 2 || job.Failures < 2 || job.PageFailures != 0 || job.ValidationOutput != "" || len(fixture.view(test).Removed) != 0 {
		test.Fatalf("page repair or preparation isolation failed: %+v", job)
	}
	attemptRecords, operationError := fixture.plugin.database.List(context.Background(), fixture.session.InstanceID, "attempt", "", 100)
	testutil.RequireNoError(test, operationError)
	rejected := 0
	for _, record := range attemptRecords {
		if strings.Contains(string(record), "validation_error") {
			rejected++
		}
	}
	if rejected < 2 {
		test.Fatal("rejected worker outputs were not retained as diagnostics")
	}
}

func TestUnrepairableEvidenceCannotPublishOrRemoveSources(test *testing.T) {
	fixture := newPluginFixture(test)
	fixture.prepare(test, fixtureMessages(fixture.session, 1000), 200000)
	fixture.services.mutex.Lock()
	fixture.services.respond = func(context.Context, harness.WorkspaceAgentRequest) (harness.WorkspaceAgentResponse, error) {
		return harness.WorkspaceAgentResponse{Text: `{"memories":[{"title":"Invalid","categories":["coding"],"compressed":{"low":"Unsupported fact","medium":"Unsupported fact","high":"Unsupported"},"kind":"user_fact","important":false,"evidence":[{"source_id":999999,"quote":"fabricated"}],"supersedes":[]}]}`}, nil
	}
	fixture.services.mutex.Unlock()
	dropMessages(test, fixture, []int64{fixture.view(test).Sources["decision"]}, true)
	job := waitForJob(test, fixture, "failed")
	if job.ValidationOutput == "" || len(job.Proposals) != 0 || len(fixture.view(test).Removed) != 0 {
		test.Fatal("unvalidated worker output changed durable memory or context")
	}
	memories, operationError := fixture.plugin.database.List(context.Background(), fixture.session.InstanceID, "memory", "", 20)
	testutil.RequireNoError(test, operationError)
	if len(memories) != 0 {
		test.Fatal("invalid evidence published a memory")
	}
}
