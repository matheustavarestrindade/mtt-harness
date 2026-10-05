package loop_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
	"github.com/matheustavarestrindade/mtt-harness/internal/tools"
)

func TestFileEditFeedbackAndRecoveryReachProviderRequests(test *testing.T) {
	type wireRequest struct {
		Messages []struct {
			Role       string `json:"role"`
			ToolCallID string `json:"tool_call_id"`
			Content    string `json:"content"`
		} `json:"messages"`
		Tools []struct {
			Function struct {
				Name, Description string
				Parameters        json.RawMessage `json:"parameters"`
			} `json:"function"`
		} `json:"tools"`
	}
	largeInput, operationError := json.Marshal(map[string]any{"path": "large.txt", "actions": []any{map[string]any{"op": "write", "content": strings.Repeat("row\n", 250)}}, "return": map[string]any{"type": "read"}})
	testutil.RequireNoError(test, operationError)
	calls := []atom.ToolCall{
		{ID: "retired", Name: "read", Input: []byte(`{"path":"file.txt"}`)},
		{ID: "create", Name: "file_actions", Input: []byte(`{"path":"file.txt","actions":[{"op":"write","content":"one\ntwo\n"}],"return":{"type":"summary"}}`)},
		{ID: "failed", Name: "file_actions", Input: []byte(`{"path":"file.txt","actions":[{"op":"append","content":"UNCOMMITTED\n"},{"op":"replace","old_text":"absent","new_text":"three"}],"return":{"type":"summary"},"on_error":{"return":{"type":"read"}}}`)},
		{ID: "recover", Name: "file_actions", Input: []byte(`{"path":"file.txt","actions":[{"op":"replace","old_text":"two","new_text":"three"}],"return":{"type":"diff","context_lines":0}}`)},
		{ID: "preview", Name: "file_actions", Input: []byte(`{"path":"file.txt","actions":[{"op":"write","content":"one\nthree\n"}],"return":{"type":"read","start_line":2,"end_line":2}}`)},
		{ID: "large", Name: "file_actions", Input: largeInput},
		{ID: "continue", Name: "file_actions", Input: []byte(`{"path":"large.txt","actions":[{"op":"read","start_line":201}]}`)},
	}
	requests := make(chan wireRequest, len(calls)+1)
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		var payload wireRequest
		if operationError := json.NewDecoder(request.Body).Decode(&payload); operationError != nil {
			test.Error(operationError)
			http.Error(responseWriter, "invalid request", http.StatusBadRequest)
			return
		}
		requests <- payload
		responseWriter.Header().Set("Content-Type", "text/event-stream")
		index := int(requestCount.Add(1)) - 1
		if index >= len(calls) {
			fmt.Fprint(responseWriter, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			return
		}
		call := calls[index]
		data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{
			"delta": map[string]any{"tool_calls": []any{map[string]any{
				"index": 0, "id": call.ID, "function": map[string]any{"name": call.Name, "arguments": string(call.Input)},
			}}}, "finish_reason": "tool_calls",
		}}})
		fmt.Fprintf(responseWriter, "data: %s\n\ndata: [DONE]\n\n", data)
	}))
	defer server.Close()
	testStack := newStack(test)
	modelProvider := provider.New(atom.ProviderSpec{Name: "wire", APIURL: server.URL})
	modelProvider.SetModels([]atom.ModelInfo{{ID: "edit-model", Input: []atom.MediaType{atom.Text}, Tools: true, ContextMax: 128000}})
	testStack.harnessRuntime.Provider(modelProvider)
	for _, tool := range []harness.Tool{tools.NewSearch(testStack.registry), tools.NewFileActions(false)} {
		testutil.RequireNoError(test, testStack.registry.Add(tool))
	}
	session := testStack.instance(test, 2)
	testStack.selectModel(test, &session, "wire/edit-model")
	testStack.user(test, session, "create and edit a file")
	testutil.RequireNoError(test, testStack.loop.Run(context.Background(), session))
	if requestCount.Load() != int32(len(calls)+1) {
		test.Fatalf("tool failure interrupted the model loop: %d requests", requestCount.Load())
	}
	feedback := map[string]string{}
	definitions := map[string]string{}
	schemas := map[string]json.RawMessage{}
	for requestIndex := range len(calls) + 1 {
		payload := <-requests
		for _, message := range payload.Messages {
			if message.Role == "tool" {
				feedback[message.ToolCallID] = message.Content
			}
		}
		for _, definition := range payload.Tools {
			if definition.Function.Name == "read" || definition.Function.Name == "write" || definition.Function.Name == "replace" {
				test.Fatalf("retired definition sent to provider: %s", definition.Function.Name)
			}
			definitions[definition.Function.Name] = definition.Function.Description
			schemas[definition.Function.Name] = definition.Function.Parameters
		}
		if requestIndex == 0 && schemas["file_actions"] == nil {
			test.Fatal("file_actions schema was not present in the first request")
		}
	}
	for identifier, fragments := range map[string][]string{
		"retired":  {"read is retired", "file_actions", "actions array"},
		"create":   {"Created", "Lines: 0 -> 2"},
		"failed":   {"action 2 (replace)", "old_text was not found", "This call committed no file changes", "Recovery read", "one\ntwo\n"},
		"recover":  {"Updated", "replace: 1 match(es)", "-two\n+three\n"},
		"preview":  {"Unchanged", "Return read (final file):\nthree\n"},
		"large":    {"Shared file_actions preview budget reached", `"start_line":201`},
		"continue": {strings.Repeat("row\n", 50)},
	} {
		for _, fragment := range fragments {
			if !strings.Contains(feedback[identifier], fragment) {
				test.Fatalf("provider feedback %q lacks %q: %s", identifier, fragment, feedback[identifier])
			}
		}
	}
	if feedback["continue"] != strings.Repeat("row\n", 50) || strings.Contains(feedback["preview"], "@@") || strings.Count(feedback["large"], "row\n") != 200 || strings.Contains(feedback["failed"], "UNCOMMITTED") || strings.Contains(feedback["create"], "+one") {
		test.Fatal("provider received duplicate/full-file output instead of the selected preview and continuation")
	}
	var actionSchema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	testutil.RequireNoError(test, json.Unmarshal(schemas["file_actions"], &actionSchema))
	var returnSchema struct {
		Description string          `json:"description"`
		Default     json.RawMessage `json:"default"`
		Properties  map[string]struct {
			Description string   `json:"description"`
			Enum        []string `json:"enum"`
		} `json:"properties"`
	}
	testutil.RequireNoError(test, json.Unmarshal(actionSchema.Properties["return"], &returnSchema))
	if returnSchema.Description == "" || returnSchema.Default != nil || strings.Join(returnSchema.Properties["type"].Enum, ",") != "summary,diff,read,list" {
		test.Fatalf("provider request lost explicit output choices: %s", schemas["file_actions"])
	}
	for _, name := range []string{"type", "context_lines", "start_line", "end_line", "start_byte", "limit", "cursor"} {
		if returnSchema.Properties[name].Description == "" {
			test.Fatalf("return.%s has no model-facing instructions", name)
		}
	}
	if !strings.Contains(definitions["file_actions"], "Never returns a diff by default") || actionSchema.Properties["on_error"] == nil || actionSchema.Properties["actions"] == nil {
		test.Fatalf("provider definition lost file-action instructions: %s", schemas["file_actions"])
	}
	instance, found := testStack.instances.Get(session.InstanceID)
	if !found {
		test.Fatal("instance missing")
	}
	content, operationError := os.ReadFile(filepath.Join(instance.Spec().Workspace, "file.txt"))
	testutil.RequireNoError(test, operationError)
	if string(content) != "one\nthree\n" {
		test.Fatalf("recovered edit has incorrect file content: %q", content)
	}
}
