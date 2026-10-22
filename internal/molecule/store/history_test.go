package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/postgres"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestBoundedHistoryStoreContract(test *testing.T) {
	factories := map[string]func(*testing.T) store.Store{"memory": func(*testing.T) store.Store { return memory.New() }, "postgres": func(test *testing.T) store.Store {
		url := os.Getenv("MTT_TEST_DATABASE_URL")
		if url == "" {
			test.Skip("PostgreSQL test database is not configured")
		}
		database, operationError := postgres.Open(context.Background(), url)
		testutil.RequireNoError(test, operationError)
		test.Cleanup(func() { database.Close() })
		return database
	}}
	for name, factory := range factories {
		test.Run(name, func(test *testing.T) { verifyBoundedHistory(test, factory(test)) })
	}
}

func verifyBoundedHistory(test *testing.T, database store.Store) {
	operationContext := context.Background()
	session := atom.Session{ID: atom.SessionID(fmt.Sprintf("history-%d", time.Now().UnixNano())), InstanceID: "history-workspace", CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	test.Cleanup(func() {
		_, operationError := database.Sessions().DeleteConversation(context.Background(), session.ID)
		testutil.RequireNoError(test, operationError)
	})
	childID := atom.SessionID(string(session.ID) + "-child")
	for _, event := range []atom.Event{
		{Name: atom.EventModelCall, Payload: json.RawMessage(`{"message_id":"known-origin"}`)},
		{Name: atom.EventAgentStart, Payload: json.RawMessage(fmt.Sprintf(`{"session":%q,"task":"Repeated task text"}`, childID))},
		{Name: atom.EventModelCall, Payload: json.RawMessage(`{"message_id":"later-response"}`)},
	} {
		event.SessionID = session.ID
		event.InstanceID = session.InstanceID
		event.Time = time.Now()
		testutil.RequireNoError(test, database.Events().Append(operationContext, event))
	}
	origins, operationError := database.Events().AgentOrigins(operationContext, session.InstanceID, []atom.SessionID{childID})
	testutil.RequireNoError(test, operationError)
	if len(origins) != 1 || origins[0].MessageID != "known-origin" || origins[0].Parent != session.ID {
		test.Fatalf("legacy origin did not follow recorded event order: %+v", origins)
	}
	for index := 0; index < 240; index++ {
		message := atom.Message{ID: fmt.Sprintf("%s-%d", session.ID, index), SessionID: session.ID, Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: fmt.Sprintf("message %d", index)}}, CreatedAt: time.Now()}
		testutil.RequireNoError(test, database.Sessions().Append(operationContext, message))
	}
	owner := atom.Message{ID: string(session.ID) + "-owner", SessionID: session.ID, Role: atom.RoleAssistant, Content: []atom.Content{{Type: atom.Text, Text: "original plan"}}, ToolCalls: []atom.ToolCall{{ID: "first", Name: "bash", Input: json.RawMessage(`{"command":"one"}`)}, {ID: "second", Name: "bash", Input: json.RawMessage(`{"command":"two"}`)}}, Usage: &atom.Usage{Input: 100, CacheRead: 30, Output: 20, Reasoning: 7}, ProviderState: &atom.ProviderState{Provider: "synthetic", Reasoning: []json.RawMessage{json.RawMessage(`{"encrypted_content":"PRIVATE_CIPHERTEXT"}`)}}, CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Sessions().Append(operationContext, owner))
	testutil.RequireNoError(test, database.Usage().Save(operationContext, atom.UsageRecord{InstanceID: session.InstanceID, SessionID: session.ID, RequestID: owner.ID, ModelID: "fixture/history-model", Usage: *owner.Usage, CreatedAt: time.Now()}))
	for _, identifier := range []string{"first", "second"} {
		testutil.RequireNoError(test, database.Sessions().Append(operationContext, atom.Message{ID: string(session.ID) + identifier, SessionID: session.ID, Role: atom.RoleTool, ToolCallID: identifier, Content: []atom.Content{{Type: atom.Text, Text: identifier + " output"}}, CreatedAt: time.Now()}))
	}
	page, operationError := database.Sessions().History(operationContext, session.ID, store.HistoryQuery{})
	testutil.RequireNoError(test, operationError)
	if len(page.Messages) != 50 || !page.HasOlder || page.HasNewer || page.LatestResponse == nil || page.LatestResponse.Model != "fixture/history-model" {
		test.Fatalf("tail metadata: %+v", page)
	}
	if page.Messages[0].Seq <= 0 || page.LastSeq != page.HeadSeq {
		test.Fatal("saved sequence positions are unavailable")
	}
	public, operationError := json.Marshal(page)
	testutil.RequireNoError(test, operationError)
	if strings.Contains(string(public), "PRIVATE_CIPHERTEXT") {
		test.Fatal("history page exposed private provider continuation")
	}
	one, operationError := database.Sessions().History(operationContext, session.ID, store.HistoryQuery{Limit: 1})
	testutil.RequireNoError(test, operationError)
	if len(one.Messages) != 1 || len(one.Context) != 1 || len(one.Context[0].ToolCalls) != 1 || one.Context[0].ToolCalls[0].ID != "second" || len(one.Context[0].Content) != 0 {
		test.Fatalf("tool boundary did not retain an exact sparse owner: %+v", one)
	}
	seen := map[string]bool{}
	for _, message := range page.Messages {
		seen[message.ID] = true
	}
	cursor := page.FirstSeq
	for page.HasOlder {
		page, operationError = database.Sessions().History(operationContext, session.ID, store.HistoryQuery{Before: cursor, Limit: 50, Revision: &page.Revision})
		testutil.RequireNoError(test, operationError)
		if len(page.Messages) > 50 {
			test.Fatal("page exceeded its count")
		}
		for _, message := range page.Messages {
			if seen[message.ID] {
				test.Fatal("duplicate page boundary")
			}
			seen[message.ID] = true
		}
		cursor = page.FirstSeq
	}
	if len(seen) != 243 {
		test.Fatalf("history traversal lost messages: %d", len(seen))
	}
	if _, operationError = database.Sessions().History(operationContext, session.ID, store.HistoryQuery{Before: 3, After: 1}); operationError == nil {
		test.Fatal("ambiguous direction accepted")
	}
	oldRevision := page.Revision
	_, operationError = database.Sessions().DeleteAfter(operationContext, session.ID, string(session.ID)+"-10")
	testutil.RequireNoError(test, operationError)
	_, operationError = database.Sessions().History(operationContext, session.ID, store.HistoryQuery{Revision: &oldRevision})
	if !errors.Is(operationError, store.ErrHistoryChanged) {
		test.Fatalf("old history cursor survived revert: %v", operationError)
	}
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	current, operationError := database.Sessions().Get(operationContext, session.ID)
	testutil.RequireNoError(test, operationError)
	if current.HistoryRevision <= oldRevision {
		test.Fatal("lifecycle save restored an old history revision")
	}
	for index := 0; index < 4; index++ {
		testutil.RequireNoError(test, database.Sessions().Append(operationContext, atom.Message{ID: fmt.Sprintf("%s-large-%d", session.ID, index), SessionID: session.ID, Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: strings.Repeat("x", 700000)}}, CreatedAt: time.Now()}))
	}
	bounded, operationError := database.Sessions().History(operationContext, session.ID, store.HistoryQuery{Limit: 100})
	testutil.RequireNoError(test, operationError)
	if len(bounded.Messages) > 2 || !bounded.HasOlder {
		test.Fatal("history byte budget was ignored")
	}
	testutil.RequireNoError(test, database.Sessions().Append(operationContext, atom.Message{ID: string(session.ID) + "-oversized", SessionID: session.ID, Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: strings.Repeat("z", store.HistoryPageBytes+1)}}, CreatedAt: time.Now()}))
	bounded, operationError = database.Sessions().History(operationContext, session.ID, store.HistoryQuery{})
	testutil.RequireNoError(test, operationError)
	if len(bounded.Messages) != 1 || len(bounded.Messages[0].Content[0].Text) != store.HistoryPageBytes+1 {
		test.Fatal("oversized message was lost or truncated")
	}
	verifyTaskInputReference(test, database, session)
}

func verifyTaskInputReference(testingHandle *testing.T, database store.Store, parent atom.Session) {
	testingHandle.Helper()
	operationContext := context.Background()
	child := atom.Session{ID: atom.SessionID(string(parent.ID) + "-input-child"), Parent: parent.ID, InstanceID: parent.InstanceID, CreatedAt: time.Now()}
	testutil.RequireNoError(testingHandle, database.Sessions().Save(operationContext, child))
	text := strings.Repeat("Literal input 🌍.\n", 3000) + "FINAL INPUT MARKER"
	input := atom.Message{ID: string(child.ID) + "-input", SessionID: child.ID, Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: text}}, ProviderState: &atom.ProviderState{Provider: "private", ChatReasoning: "PRIVATE_TASK_STATE"}, CreatedAt: time.Now()}
	testutil.RequireNoError(testingHandle, database.Sessions().Append(operationContext, input))
	for index := 0; index < 130; index++ {
		testutil.RequireNoError(testingHandle, database.Sessions().Append(operationContext, atom.Message{ID: fmt.Sprintf("%s-result-%d", child.ID, index), SessionID: child.ID, Role: atom.RoleAssistant, Content: []atom.Content{{Type: atom.Text, Text: "later output"}}, CreatedAt: time.Now()}))
	}
	metadata, operationError := database.Sessions().History(operationContext, child.ID, store.HistoryQuery{MetadataOnly: true})
	testutil.RequireNoError(testingHandle, operationError)
	if metadata.TaskMessageID != input.ID || metadata.TaskSeq <= 0 || len(metadata.Messages) != 0 || len(metadata.Task) >= len(text) {
		testingHandle.Fatalf("task reference did not retain bounded metadata: id=%q seq=%d messages=%d", metadata.TaskMessageID, metadata.TaskSeq, len(metadata.Messages))
	}
	page, operationError := database.Sessions().History(operationContext, child.ID, store.HistoryQuery{Before: metadata.TaskSeq + 1, Limit: 1, Revision: &metadata.Revision})
	testutil.RequireNoError(testingHandle, operationError)
	if len(page.Messages) != 1 || page.Messages[0].ID != input.ID || page.Messages[0].Content[0].Text != text || page.Messages[0].ProviderState != nil {
		testingHandle.Fatal("direct task input read changed or exposed its source")
	}
	public, operationError := json.Marshal(page)
	testutil.RequireNoError(testingHandle, operationError)
	if strings.Contains(string(public), "PRIVATE_TASK_STATE") {
		testingHandle.Fatal("task read exposed private continuation")
	}
	oldRevision := metadata.Revision
	_, operationError = database.Sessions().DeleteAfter(operationContext, child.ID, input.ID)
	testutil.RequireNoError(testingHandle, operationError)
	_, operationError = database.Sessions().History(operationContext, child.ID, store.HistoryQuery{Before: metadata.TaskSeq + 1, Limit: 1, Revision: &oldRevision})
	if !errors.Is(operationError, store.ErrHistoryChanged) {
		testingHandle.Fatal("old task reference survived a history revision")
	}
}
