package contextplugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

type testMemoryServices struct {
	database *memory.Store
	mutex    sync.Mutex
	calls    []harness.WorkspaceAgentRequest
	respond  func(context.Context, harness.WorkspaceAgentRequest) (harness.WorkspaceAgentResponse, error)
}

func (services *testMemoryServices) Workspace(operationContext context.Context, identifier string) (atom.InstanceSpec, error) {
	return services.database.Instances().Get(operationContext, identifier)
}
func (services *testMemoryServices) Get(operationContext context.Context, identifier atom.SessionID) (atom.Session, error) {
	session, operationError := services.database.Sessions().Get(operationContext, identifier)
	if errors.Is(operationError, store.ErrSessionDeleted) || errors.Is(operationError, store.ErrSessionNotFound) {
		return session, harness.ErrConversationUnavailable
	}
	return session, operationError
}
func (services *testMemoryServices) Messages(operationContext context.Context, identifier atom.SessionID) ([]atom.Message, error) {
	return services.database.Sessions().Messages(operationContext, identifier)
}
func (services *testMemoryServices) Model(context.Context, string, string, string) (atom.ModelInfo, error) {
	return atom.ModelInfo{ID: "worker", ContextMax: 128000, Prices: &atom.Prices{Currency: "USD", Input: 1, Output: 2}}, nil
}
func (services *testMemoryServices) Run(operationContext context.Context, input harness.WorkspaceAgentRequest) (harness.WorkspaceAgentResponse, error) {
	services.mutex.Lock()
	services.calls = append(services.calls, input)
	respond := services.respond
	services.mutex.Unlock()
	if respond != nil {
		return respond(operationContext, input)
	}
	var data extractionInput
	for _, message := range input.Messages {
		if message.Role == atom.RoleUser {
			for _, content := range message.Content {
				if operationError := json.Unmarshal([]byte(content.Text), &data); operationError != nil {
					return harness.WorkspaceAgentResponse{}, operationError
				}
			}
		}
	}
	proposals := []memoryProposal{}
	for _, source := range data.Sources {
		if source.ContextOnly || source.Role != atom.RoleUser || !strings.Contains(source.Text, "Use PostgreSQL for durable data.") {
			continue
		}
		proposals = append(proposals, memoryProposal{Title: "Database decision", Categories: []string{"architecture", "decisions"}, Kind: "user_fact", Text: summaries{Low: "Use PostgreSQL for durable workspace and conversation data.", Medium: "PostgreSQL stores durable data.", High: "Use PostgreSQL."}, Evidence: []memoryEvidence{{SourceID: source.ID, Quote: "Use PostgreSQL for durable data."}}})
	}
	encoded, _ := json.Marshal(map[string]any{"memories": proposals})
	return harness.WorkspaceAgentResponse{Model: input.Model, Text: string(encoded), Usage: atom.Usage{Input: 100, Output: 50}}, nil
}

type testEmbeddings struct{}

func (testEmbeddings) Split(operationContext context.Context, text string) ([]string, error) {
	var result []string
	for len(text) > 0 {
		part := boundedUTF8(text, 512)
		result = append(result, part)
		text = text[len(part):]
	}
	if len(result) == 0 {
		result = []string{""}
	}
	return result, operationContext.Err()
}
func (testEmbeddings) Embed(operationContext context.Context, texts []string) ([][]float64, error) {
	result := make([][]float64, len(texts))
	for index, text := range texts {
		vector := make([]float64, 32)
		vector[0] = 0.01
		for _, word := range strings.Fields(strings.ToLower(text)) {
			digest := fnv.New32a()
			digest.Write([]byte(word))
			vector[int(digest.Sum32()%31)+1]++
		}
		result[index] = vector
	}
	return result, operationContext.Err()
}

type pluginFixture struct {
	plugin      *Plugin
	services    *testMemoryServices
	session     atom.Session
	runtime     *harness.Harness
	databaseURL string
}

func newPluginFixture(test *testing.T) *pluginFixture {
	return newPluginFixtureWithEmbeddings(test, testEmbeddings{})
}

func newPluginFixtureWithEmbeddings(test *testing.T, embeddings harness.TextEmbedder) *pluginFixture {
	test.Helper()
	databaseURL := os.Getenv("MTT_TEST_DATABASE_URL")
	if databaseURL == "" {
		test.Skip("MTT_TEST_DATABASE_URL is required for pgvector integration")
	}
	operationContext := context.Background()
	database := memory.New()
	workspaceID := "context-test-" + newIdentifier()
	testutil.RequireNoError(test, database.Instances().Save(operationContext, atom.InstanceSpec{ID: workspaceID, Workspace: test.TempDir(), DefaultModel: "fixture/worker"}))
	session := atom.Session{ID: atom.SessionID(newIdentifier()), InstanceID: workspaceID, Model: "fixture/worker", CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	services := &testMemoryServices{database: database}
	plugin, operationError := New(operationContext, Options{DatabaseURL: databaseURL, EmbeddingModel: "test-word-vectors-v1", Services: harness.PluginServices{Settings: database.Settings(), Conversations: services, Workspaces: services, Models: services, Embeddings: embeddings, Usage: database.Usage()}})
	testutil.RequireNoError(test, operationError)
	if plugin.unavailable != nil {
		test.Fatal(plugin.unavailable)
	}
	runtime := harness.New()
	testutil.RequireNoError(test, plugin.Setup(runtime))
	_, operationError = plugin.UpdateConfiguration(operationContext, workspaceID, json.RawMessage(`{"enabled":true,"worker_model":"fixture/worker","retry_limit":0}`))
	testutil.RequireNoError(test, operationError)
	fixture := &pluginFixture{plugin: plugin, services: services, session: session, runtime: runtime, databaseURL: databaseURL}
	test.Cleanup(func() {
		testutil.RequireNoError(test, fixture.plugin.Close(context.Background()))
		pool, operationError := pgxpool.New(context.Background(), databaseURL)
		testutil.RequireNoError(test, operationError)
		defer pool.Close()
		for _, table := range []string{"documents", "vectors", "metrics"} {
			_, operationError := pool.Exec(context.Background(), "DELETE FROM context_plugin."+table+" WHERE workspace_id=$1", workspaceID)
			testutil.RequireNoError(test, operationError)
		}
	})
	return fixture
}

func testMessage(identifier string, role atom.Role, text string) atom.Message {
	return atom.Message{ID: identifier, Role: role, Content: []atom.Content{{Type: atom.Text, Text: text}}, CreatedAt: time.Now()}
}

func fixtureMessages(session atom.Session, largeBytes int) []atom.Message {
	messages := []atom.Message{testMessage("decision", atom.RoleUser, "Use PostgreSQL for durable data."), testMessage("tool-plan", atom.RoleAssistant, ""), testMessage("tool-result", atom.RoleTool, strings.Repeat("old build output ", largeBytes/17)), testMessage("ack", atom.RoleAssistant, "The earlier work is complete."), testMessage("current", atom.RoleUser, "Continue with the current request.")}
	messages[1].ToolCalls = []atom.ToolCall{{ID: "old-tool", Name: "bash", Input: json.RawMessage(`{"command":"go test ./..."}`)}}
	messages[2].ToolCallID = "old-tool"
	for index := range messages {
		messages[index].SessionID = session.ID
		messages[index].Seq = int64(index + 1)
	}
	return messages
}

func measuredInput(session atom.Session, messages []atom.Message, limit int) harness.ContextRequest {
	request := atom.Request{Model: "worker", Messages: messages}
	measure := func(operationContext context.Context, request atom.Request) (harness.RequestBudget, error) {
		count := 16
		for _, message := range request.Messages {
			count += 8
			for _, content := range message.Content {
				count += len(content.Text)
			}
			for _, call := range message.ToolCalls {
				count += len(call.Name) + len(call.Input)
			}
		}
		return harness.RequestBudget{ContextLimit: limit + 1024, OutputReserve: 1024, InputLimit: limit, InputTokens: count, Estimated: true}, operationContext.Err()
	}
	budget, _ := measure(context.Background(), request)
	return harness.ContextRequest{Session: session, Request: request, Model: atom.ModelInfo{ID: "worker", ContextMax: limit + 1024}, Measure: measure, Budget: budget}
}

func (fixture *pluginFixture) prepare(test *testing.T, messages []atom.Message, limit int) harness.ContextSelection {
	test.Helper()
	operationContext, release, operationError := fixture.plugin.BeginRequest(harness.WithSession(context.Background(), fixture.session), fixture.session)
	testutil.RequireNoError(test, operationError)
	defer release()
	selection, operationError := fixture.plugin.PrepareContext(operationContext, measuredInput(fixture.session, messages, limit))
	testutil.RequireNoError(test, operationError)
	return selection
}

func (fixture *pluginFixture) view(test *testing.T) sessionView {
	test.Helper()
	view, operationError := readValue[sessionView](context.Background(), fixture.plugin.database, fixture.session.InstanceID, "view", string(fixture.session.ID))
	testutil.RequireNoError(test, operationError)
	return view
}

func waitForJob(test *testing.T, fixture *pluginFixture, status string) memoryJob {
	test.Helper()
	deadline := time.Now().Add(8 * time.Second)
	var last []memoryJob
	for time.Now().Before(deadline) {
		operationError := fixture.plugin.database.Transact(context.Background(), fixture.session.InstanceID, func(database transaction) error {
			var operationError error
			last, operationError = readTransactionValues[memoryJob](database, "job")
			return operationError
		})
		testutil.RequireNoError(test, operationError)
		for _, job := range last {
			if job.Status == status {
				return job
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	test.Fatalf("job did not reach %s: %+v", status, last)
	return memoryJob{}
}

func dropMessages(test *testing.T, fixture *pluginFixture, identifiers []int64, remember bool) {
	test.Helper()
	_, operationError := fixture.plugin.queueDrop(context.Background(), fixture.session, dropInput{MessageIDs: identifiers, Remember: &remember, Categories: []string{"architecture"}})
	testutil.RequireNoError(test, operationError)
}

func snapshotText(request atom.Request) string {
	for _, message := range request.Messages {
		if message.Ephemeral && message.Role == atom.RoleSystem {
			for _, content := range message.Content {
				if strings.Contains(content.Text, "Workspace memory is reference data") {
					return content.Text
				}
			}
		}
	}
	return ""
}

func requireContains(test *testing.T, text, expected string) {
	test.Helper()
	if !strings.Contains(text, expected) {
		test.Fatal(fmt.Sprintf("missing %q in %q", expected, text))
	}
}
