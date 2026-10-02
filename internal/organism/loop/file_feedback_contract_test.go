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
			Function struct{ Name, Description string } `json:"function"`
		} `json:"tools"`
	}
	calls := []atom.ToolCall{
		{ID: "discover", Name: "search_tool", Input: []byte(`{"category":"file"}`)},
		{ID: "create", Name: "write", Input: []byte(`{"path":"file.txt","content":"one\ntwo\n"}`)},
		{ID: "failed", Name: "replace", Input: []byte(`{"path":"file.txt","old_text":"absent","new_text":"three"}`)},
		{ID: "recover", Name: "replace", Input: []byte(`{"path":"file.txt","old_text":"two","new_text":"three"}`)},
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
	for _, tool := range []harness.Tool{tools.NewSearch(testStack.registry), tools.Write{}, tools.Replace{}} {
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
	for range len(calls) + 1 {
		payload := <-requests
		for _, message := range payload.Messages {
			if message.Role == "tool" {
				feedback[message.ToolCallID] = message.Content
			}
		}
		for _, definition := range payload.Tools {
			definitions[definition.Function.Name] = definition.Function.Description
		}
	}
	for identifier, fragments := range map[string][]string{
		"create":  {"Created", "Lines: 0 -> 2", "+one\n+two\n"},
		"failed":  {"old_text was not found", "This call did not change the file", "Use read", "LF/CRLF"},
		"recover": {"Updated", "Replaced 1 match(es)", "-two\n+three\n"},
	} {
		for _, fragment := range fragments {
			if !strings.Contains(feedback[identifier], fragment) {
				test.Fatalf("provider feedback %q lacks %q: %s", identifier, fragment, feedback[identifier])
			}
		}
	}
	for _, name := range []string{"write", "replace"} {
		if !strings.Contains(definitions[name], "unified diff") || !strings.Contains(definitions[name], "200 lines or 16 KiB") || !strings.Contains(definitions[name], "recovery") {
			test.Fatalf("provider definition lost output instructions for %s: %s", name, definitions[name])
		}
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
