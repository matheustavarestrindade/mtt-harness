package contextplugin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func seedMemory(test *testing.T, fixture *pluginFixture, title, text string, old bool) memoryRecord {
	test.Helper()
	created := time.Now().UTC()
	if old {
		created = created.Add(-200 * time.Hour)
	}
	chunks, operationError := fixture.plugin.embedText(context.Background(), fixture.session.InstanceID, "test.setup", title+" "+text)
	testutil.RequireNoError(test, operationError)
	record := memoryRecord{ID: newIdentifier(), Version: 1, Title: title, Categories: []string{"preferences"}, Text: summaries{Low: text, Medium: text, High: text}, Kind: "user_fact", Status: "active", CreatedAt: created, UpdatedAt: created}
	operationError = fixture.plugin.database.Transact(context.Background(), fixture.session.InstanceID, func(database transaction) error {
		identifier, operationError := database.NextSourceID()
		if operationError != nil {
			return operationError
		}
		record.SourceIDs = []int64{identifier}
		entry := source{ID: identifier, SessionID: fixture.session.ID, MessageID: "seed-" + record.ID, Sequence: identifier, Role: atom.RoleUser, Text: text, CreatedAt: created, Indexed: true, IndexModel: fixture.plugin.embeddingModel}
		if operationError := database.Put("source", sourceKey(identifier), entry); operationError != nil {
			return operationError
		}
		if operationError := database.ReplaceVectors("source", sourceKey(identifier), 1, fixture.plugin.embeddingModel, record.Categories, chunks, false, false); operationError != nil {
			return operationError
		}
		if operationError := database.Put("memory", record.ID, record); operationError != nil {
			return operationError
		}
		if operationError := database.Put("memory_version", record.ID+":1", record); operationError != nil {
			return operationError
		}
		return database.ReplaceVectors("memory", record.ID, 1, fixture.plugin.embeddingModel, record.Categories, chunks, false, false)
	})
	testutil.RequireNoError(test, operationError)
	return record
}

func TestHistorianConsolidatesColdMemoriesButKeepsSpecificRetrieval(test *testing.T) {
	fixture := newPluginFixture(test)
	fixture.services.mutex.Lock()
	fixture.services.respond = func(operationContext context.Context, input harness.WorkspaceAgentRequest) (harness.WorkspaceAgentResponse, error) {
		var data struct {
			Memories []memoryRecord `json:"memories"`
		}
		for _, message := range input.Messages {
			if message.Role == atom.RoleUser {
				if operationError := json.Unmarshal([]byte(message.Content[0].Text), &data); operationError != nil {
					return harness.WorkspaceAgentResponse{}, operationError
				}
			}
		}
		var children []string
		for _, record := range data.Memories {
			children = append(children, record.ID)
		}
		encoded, _ := json.Marshal(map[string]any{"idea": historianIdea{Title: "Food preferences", Categories: []string{"preferences"}, Text: summaries{Low: "The user likes pizza, fries, Coke and Pepsi as fast food preferences.", Medium: "The user likes fast food and soft drinks.", High: "The user likes fast food."}, Children: children}})
		return harness.WorkspaceAgentResponse{Model: input.Model, Text: string(encoded)}, operationContext.Err()
	}
	fixture.services.mutex.Unlock()
	var records []memoryRecord
	for _, food := range []string{"pizza", "fries", "Coke", "Pepsi"} {
		records = append(records, seedMemory(test, fixture, "Food preference", "The user likes fast food: "+food+".", true))
	}
	testutil.RequireNoError(test, fixture.plugin.database.Transact(context.Background(), fixture.session.InstanceID, func(database transaction) error {
		return database.Put("historian_state", "schedule", historianSchedule{})
	}))
	configuration, _, operationError := fixture.plugin.loadConfiguration(context.Background(), fixture.session.InstanceID)
	testutil.RequireNoError(test, operationError)
	testutil.RequireNoError(test, fixture.plugin.scheduleHistorian(context.Background(), fixture.session.InstanceID, configuration))
	fixture.plugin.signalWork()
	job := waitForJob(test, fixture, "applied")
	if job.Agent != "context.historian" || len(job.ResultIDs) != 1 {
		test.Fatalf("unexpected historian result: %+v", job)
	}
	idea, operationError := readValue[memoryRecord](context.Background(), fixture.plugin.database, fixture.session.InstanceID, "memory", job.ResultIDs[0])
	testutil.RequireNoError(test, operationError)
	if idea.Kind != "idea" || len(idea.Children) != 4 {
		test.Fatalf("missing consolidation links: %+v", idea)
	}
	for _, record := range records {
		current, operationError := readValue[memoryRecord](context.Background(), fixture.plugin.database, fixture.session.InstanceID, "memory", record.ID)
		testutil.RequireNoError(test, operationError)
		if current.Status != "consolidated" || current.Retrievals != 0 {
			test.Fatal("historian erased detail or counted its own reads as user retrieval")
		}
	}
	selection := fixture.prepare(test, []atom.Message{testMessage("new-chat", atom.RoleUser, "Hello")}, 200000)
	requireContains(test, snapshotText(selection.Request), "I The user likes fast food.")
	reply, operationError := fixture.plugin.searchMemory(context.Background(), fixture.session, memorySearchInput{Query: "Pepsi", Compression: "low", Categories: []string{"preferences"}})
	testutil.RequireNoError(test, operationError)
	found := false
	for _, match := range reply.Matches {
		found = found || strings.Contains(match.Data, "Pepsi") && !match.Historical
	}
	if !found {
		test.Fatal("normal query could not recover consolidated detail")
	}
}

func TestRememberCorrectionKeepsOldVersionAndFreezesCurrentSnapshot(test *testing.T) {
	fixture := newPluginFixture(test)
	old := seedMemory(test, fixture, "Drink preference", "The user likes Pepsi.", false)
	messages := []atom.Message{testMessage("original", atom.RoleUser, "Hello")}
	first := fixture.prepare(test, messages, 200000)
	fixture.services.mutex.Lock()
	fixture.services.respond = func(operationContext context.Context, input harness.WorkspaceAgentRequest) (harness.WorkspaceAgentResponse, error) {
		var data extractionInput
		for _, message := range input.Messages {
			if message.Role == atom.RoleUser {
				if operationError := json.Unmarshal([]byte(message.Content[0].Text), &data); operationError != nil {
					return harness.WorkspaceAgentResponse{}, operationError
				}
			}
		}
		var sourceID int64
		for _, source := range data.Sources {
			if source.Role == atom.RoleUser && strings.Contains(source.Text, "I now dislike Pepsi.") {
				sourceID = source.ID
			}
		}
		proposal := memoryProposal{Title: "Drink preference", Categories: []string{"preferences"}, Text: summaries{Low: "The user now dislikes Pepsi.", Medium: "The user dislikes Pepsi.", High: "Dislikes Pepsi."}, Kind: "user_fact", Evidence: []memoryEvidence{{SourceID: sourceID, Quote: "I now dislike Pepsi."}}, Supersedes: []string{old.ID}}
		encoded, _ := json.Marshal(map[string]any{"memories": []memoryProposal{proposal}})
		return harness.WorkspaceAgentResponse{Model: input.Model, Text: string(encoded)}, operationContext.Err()
	}
	fixture.services.mutex.Unlock()
	messages = append(messages, testMessage("answer", atom.RoleAssistant, "Understood."), testMessage("correction", atom.RoleUser, "I now dislike Pepsi."))
	fixture.prepare(test, messages, 200000)
	_, operationError := fixture.plugin.queueRemember(context.Background(), fixture.session, "remember-correction", rememberInput{Text: "The user now dislikes Pepsi.", Categories: []string{"preferences"}})
	testutil.RequireNoError(test, operationError)
	waitForJob(test, fixture, "applied")
	current, operationError := readValue[memoryRecord](context.Background(), fixture.plugin.database, fixture.session.InstanceID, "memory", old.ID)
	testutil.RequireNoError(test, operationError)
	if current.Version != 2 {
		test.Fatalf("correction did not version the existing memory: %+v", current)
	}
	storedOld, operationError := readValue[memoryRecord](context.Background(), fixture.plugin.database, fixture.session.InstanceID, "memory_version", old.ID+":1")
	testutil.RequireNoError(test, operationError)
	if storedOld.Text.High != old.Text.High {
		test.Fatal("correction erased the old version")
	}
	unchanged := fixture.prepare(test, messages, 200000)
	if snapshotText(first.Request) != snapshotText(unchanged.Request) {
		test.Fatal("remember rewrote the cached memory block")
	}
	currentReply, operationError := fixture.plugin.searchMemory(context.Background(), fixture.session, memorySearchInput{Query: "Pepsi", Compression: "low"})
	testutil.RequireNoError(test, operationError)
	for _, match := range currentReply.Matches {
		if strings.Contains(match.Data, "likes Pepsi") && !strings.Contains(match.Data, "dislikes Pepsi") {
			test.Fatal("old preference was returned as current")
		}
	}
	historyReply, operationError := fixture.plugin.searchMemory(context.Background(), fixture.session, memorySearchInput{Query: "Pepsi", Compression: "low", IncludeHistory: true})
	testutil.RequireNoError(test, operationError)
	historical := false
	for _, match := range historyReply.Matches {
		historical = historical || match.Historical && match.Data == old.Text.Low
	}
	if !historical {
		test.Fatal("old preference cannot be queried as history")
	}
	view := fixture.view(test)
	dropMessages(test, fixture, []int64{view.Sources["original"]}, false)
	waitForJob(test, fixture, "ready")
	_, operationError = fixture.plugin.requestWrapup(context.Background(), fixture.session)
	testutil.RequireNoError(test, operationError)
	refreshed := fixture.prepare(test, messages, 200000)
	requireContains(test, snapshotText(refreshed.Request), "L The user now dislikes Pepsi.")
	if fixture.view(test).Snapshot[0].Version != 2 {
		test.Fatal("refresh did not install the correction")
	}
}
