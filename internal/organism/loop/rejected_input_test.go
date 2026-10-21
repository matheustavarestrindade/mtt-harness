package loop_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/eventbus"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/permission"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/registry"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
	"github.com/matheustavarestrindade/mtt-harness/internal/tools"
)

func TestCompletedInvalidJSONIsReturnedToModelWithoutExecutionOrReplay(testingHandle *testing.T) {
	operationContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var requests atomic.Int32
	var mutex sync.Mutex
	var checked, executed, planned []string
	malformed := `{"value":`
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		var payload map[string]any
		testutil.RequireNoError(testingHandle, json.NewDecoder(request.Body).Decode(&payload))
		number := requests.Add(1)
		responseWriter.Header().Set("Content-Type", "text/event-stream")
		send := func(value any) {
			encoded, operationError := json.Marshal(value)
			testutil.RequireNoError(testingHandle, operationError)
			_, operationError = fmt.Fprintf(responseWriter, "data: %s\n\n", encoded)
			testutil.RequireNoError(testingHandle, operationError)
			responseWriter.(http.Flusher).Flush()
		}
		call := func(id, name, arguments string) map[string]any {
			return map[string]any{"type": "function_call", "id": "item-" + id, "call_id": id, "name": name, "arguments": arguments, "status": "completed"}
		}
		var output []any
		switch number {
		case 1:
			output = []any{call("discover", "search_tool", `{"query":"probe"}`)}
		case 2:
			output = []any{call("once", "probe", `{"value":"first"}`), call("rejected", "probe", malformed)}
		case 3:
			original, failed, successful := false, false, false
			for _, value := range payload["input"].([]any) {
				item := value.(map[string]any)
				if item["type"] == "function_call" && item["call_id"] == "rejected" {
					original = item["arguments"] == malformed
				}
				if item["type"] == "function_call_output" && item["call_id"] == "rejected" {
					failed = strings.Contains(item["output"].(string), "input was not executed: invalid JSON")
				}
				if item["type"] == "function_call_output" && item["call_id"] == "once" {
					successful = true
				}
			}
			if !original || !failed || !successful {
				testingHandle.Errorf("repair request lost original arguments or preceding results: original=%t failed=%t successful=%t", original, failed, successful)
			}
			output = []any{call("corrected", "probe", `{"value":"corrected"}`)}
		case 4:
			output = []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "Completed after correcting the input."}}}}
		default:
			testingHandle.Error("unexpected extra model request")
		}
		for index, item := range output {
			send(map[string]any{"type": "response.output_item.done", "output_index": index, "item": item})
		}
		send(map[string]any{"type": "response.completed", "response": map[string]any{"id": fmt.Sprintf("response-%d", number), "output": output, "usage": map[string]any{"input_tokens": 20, "output_tokens": 5}}})
	}))
	defer server.Close()
	database := memory.New()
	runtime := harness.New()
	harness.Pipe(runtime, atom.StageActionPlan, func(_ context.Context, call atom.ToolCall) (atom.ToolCall, error) {
		mutex.Lock()
		planned = append(planned, call.ID)
		mutex.Unlock()
		return call, nil
	})
	toolRegistry := registry.New(runtime, nil)
	testutil.RequireNoError(testingHandle, toolRegistry.Add(tools.NewSearch(toolRegistry)))
	testutil.RequireNoError(testingHandle, toolRegistry.Add(&fakeTool{name: "probe", inputSchema: `{}`, check: func(_ context.Context, call atom.ToolCall) atom.Verdict {
		mutex.Lock()
		checked = append(checked, call.ID)
		mutex.Unlock()
		return atom.Verdict{Kind: atom.VerdictAllow}
	}, run: func(_ context.Context, call atom.ToolCall) (atom.ToolResult, error) {
		mutex.Lock()
		executed = append(executed, call.ID)
		mutex.Unlock()
		return atom.ToolResult{Status: atom.StatusOK, Content: []atom.Content{{Type: atom.Text, Text: "Observed success."}}}, nil
	}}))
	modelProvider := provider.New(atom.ProviderSpec{Name: "synthetic", Protocol: "responses", APIURL: server.URL})
	modelProvider.SetModels([]atom.ModelInfo{{ID: "model", Tools: true, ContextMax: 128000}})
	defer modelProvider.Close()
	models := gateway.New(runtime)
	testutil.RequireNoError(testingHandle, models.Add(modelProvider))
	manager := instances.New(func(string) instances.SessionManager { return nil }, nil)
	_, operationError := manager.Start(operationContext, atom.InstanceSpec{ID: "workspace", Workspace: testingHandle.TempDir(), DefaultModel: "synthetic/model", AgentDepthLimit: 2})
	testutil.RequireNoError(testingHandle, operationError)
	agentLoop := loop.New(runtime, loop.Config{Gateway: models, Registry: toolRegistry, Store: database, Bus: eventbus.New(runtime), Broker: permission.NewBroker(), Engine: permission.NewEngine(), Instances: manager})
	session := atom.Session{ID: "session", InstanceID: "workspace", Model: "synthetic/model", CreatedAt: time.Now()}
	testutil.RequireNoError(testingHandle, database.Sessions().Save(operationContext, session))
	testutil.RequireNoError(testingHandle, database.Sessions().Append(operationContext, atom.Message{ID: "user", SessionID: session.ID, Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "Inspect the test data."}}, CreatedAt: time.Now()}))
	testutil.RequireNoError(testingHandle, agentLoop.Run(operationContext, session))
	mutex.Lock()
	defer mutex.Unlock()
	if !slices.Equal(executed, []string{"once", "corrected"}) || !slices.Equal(checked, []string{"once", "corrected"}) || requests.Load() != 4 {
		testingHandle.Fatalf("bad input executed or old call replayed: executed=%v checked=%v requests=%d", executed, checked, requests.Load())
	}
	if slices.Contains(planned, "rejected") {
		testingHandle.Fatal("rejected input reached action middleware")
	}
	messages, operationError := database.Sessions().Messages(operationContext, session.ID)
	testutil.RequireNoError(testingHandle, operationError)
	found := false
	for _, message := range messages {
		for _, call := range message.ToolCalls {
			if call.ID == "rejected" {
				text, operationError := call.ArgumentsText()
				testutil.RequireNoError(testingHandle, operationError)
				found = text == malformed && call.InputError != ""
			}
		}
	}
	if !found || !strings.Contains(messages[len(messages)-1].Content[0].Text, "Completed after correcting") {
		testingHandle.Fatal("completed correction/history was lost")
	}
}
