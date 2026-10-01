package loop_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/mcp"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
	"github.com/matheustavarestrindade/mtt-harness/internal/tools"
)

// Exercise discovery, session schema enrichment, and the actual HTTP payload.
// Static declarations alone would miss the agent metadata replacement bug.
func TestDiscoveredToolContractsReachProviderRequests(test *testing.T) {
	type parameter struct {
		Description string   `json:"description"`
		Default     any      `json:"default"`
		Enum        []string `json:"enum"`
	}
	type wireTool struct {
		Function struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Parameters  struct {
				Properties map[string]parameter `json:"properties"`
			} `json:"parameters"`
		} `json:"function"`
	}
	type wireRequest struct {
		Tools    []wireTool `json:"tools"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	requests := make(chan wireRequest, 2)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		var payload wireRequest
		if operationError := json.NewDecoder(request.Body).Decode(&payload); operationError != nil {
			test.Error(operationError)
			http.Error(responseWriter, "invalid request", http.StatusBadRequest)
			return
		}
		requests <- payload
		responseWriter.Header().Set("Content-Type", "text/event-stream")
		if calls.Add(1) == 1 {
			var toolCalls []map[string]any
			for index, category := range []string{"process", "file", "agent", "mcp"} {
				arguments, _ := json.Marshal(map[string]string{"category": category})
				toolCalls = append(toolCalls, map[string]any{"index": index, "id": "discover-" + category, "function": map[string]any{"name": "search_tool", "arguments": string(arguments)}})
			}
			data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": toolCalls}, "finish_reason": "tool_calls"}}})
			fmt.Fprintf(responseWriter, "data: %s\n\ndata: [DONE]\n\n", data)
			return
		}
		fmt.Fprint(responseWriter, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	testStack := newStack(test)
	standardProvider := provider.New(atom.ProviderSpec{Name: "wire", APIURL: server.URL})
	standardProvider.SetModels([]atom.ModelInfo{{ID: "wire-model", Input: []atom.MediaType{atom.Text}, Tools: true, ContextMax: 128000}})
	testStack.harnessRuntime.Provider(standardProvider)
	for _, tool := range []harness.Tool{
		tools.NewSearch(testStack.registry), tools.NewBash(nil), tools.Read{}, tools.Write{}, tools.Replace{},
		tools.NewProcessOutput(nil), tools.NewProcessKill(nil), &tools.Agent{RunTask: testStack.loop.RunAgentTask}, tools.Finish{},
		&mcp.Tool{Server: "remote", Spec: mcp.ToolSpec{Name: "lookup", Description: "External lookup", InputSchema: json.RawMessage(`{"type":"object","properties":{"timeout":{"type":"integer","description":"Server timeout in seconds.","default":15}}}`)}},
	} {
		testutil.RequireNoError(test, testStack.registry.Add(tool))
	}
	session := testStack.instance(test, 2)
	session.Model = "wire/wire-model"
	testutil.RequireNoError(test, testStack.database.Sessions().Save(context.Background(), session))
	testStack.user(test, session, "discover the available tools")
	testutil.RequireNoError(test, testStack.loop.Run(context.Background(), session))
	<-requests
	payload := <-requests
	definitions := map[string]wireTool{}
	for _, tool := range payload.Tools {
		definitions[tool.Function.Name] = tool
		if tool.Function.Description == "" {
			test.Errorf("tool %s lost its description", tool.Function.Name)
		}
		for name, property := range tool.Function.Parameters.Properties {
			if property.Description == "" {
				test.Errorf("%s.%s reached the model without instructions", tool.Function.Name, name)
			}
		}
	}
	for _, message := range payload.Messages {
		if message.Role != "tool" {
			continue
		}
		var references []map[string]json.RawMessage
		testutil.RequireNoError(test, json.Unmarshal([]byte(message.Content), &references))
		for _, reference := range references {
			if len(reference) != 2 || reference["Name"] == nil || reference["Categories"] == nil {
				test.Fatalf("provider history contains verbose discovery metadata: %s", message.Content)
			}
		}
	}
	if len(definitions) != 10 {
		test.Fatalf("discovered definitions: %v", definitions)
	}
	for _, name := range []string{"read", "write", "replace"} {
		properties := definitions[name].Function.Parameters.Properties
		if properties["start_line"].Description == "" || properties["end_line"].Description == "" {
			test.Fatalf("%s line-range instructions did not reach the provider", name)
		}
	}
	if properties := definitions["replace"].Function.Parameters.Properties; properties["mode"].Default != "first" || len(properties["mode"].Enum) != 3 {
		test.Fatalf("replace mode contract disappeared: %+v", properties)
	}
	bashProperties := definitions["bash"].Function.Parameters.Properties
	if !strings.Contains(bashProperties["timeout"].Description, "milliseconds") || !strings.Contains(bashProperties["interval"].Description, "milliseconds") {
		test.Fatal("bash time units did not reach the model")
	}
	if bashProperties["timeout"].Default != float64(0) || bashProperties["wait"].Default != true || bashProperties["notify"].Default != "exit" {
		test.Fatalf("bash defaults disappeared: %+v", bashProperties)
	}
	modelProperty := definitions["agent"].Function.Parameters.Properties["model"]
	if !strings.Contains(modelProperty.Description, "instance default model") || !strings.Contains(modelProperty.Description, "Available models:") || len(modelProperty.Enum) != 2 {
		test.Fatalf("model choices replaced the parameter contract: %+v", modelProperty)
	}
	if modelProperty.Default != "test/test-model" {
		test.Fatalf("agent default must be the instance default, not the parent model: %+v", modelProperty)
	}
	external := definitions["mcp__remote__lookup"].Function.Parameters.Properties["timeout"]
	if external.Description != "Server timeout in seconds." || external.Default != float64(15) {
		test.Fatalf("MCP metadata changed: %+v", external)
	}
}

func TestShellQueryEnablesBashOnTheNextModelRequest(test *testing.T) {
	testStack := newStack(test,
		provider.Call("search_tool", `{"query":"shell command exec"}`),
		provider.Text("done"),
	)
	testutil.RequireNoError(test, testStack.registry.Add(tools.NewSearch(testStack.registry)))
	testutil.RequireNoError(test, testStack.registry.Add(tools.NewBash(nil)))
	session := testStack.instance(test, 2)
	testStack.user(test, session, "find the command execution tool")
	testutil.RequireNoError(test, testStack.loop.Run(context.Background(), session))
	if len(testStack.provider.Requests) != 2 {
		test.Fatalf("discovery did not continue to the next request: %d", len(testStack.provider.Requests))
	}
	for _, toolSpec := range testStack.provider.Requests[1].Tools {
		if toolSpec.Name == "bash" {
			if toolSpec.Description == "" || len(toolSpec.InputSchema.JSON) == 0 {
				test.Fatal("compact discovery lost the callable bash instructions or schema")
			}
			return
		}
	}
	test.Fatal("the shell query did not enable bash in the next request")
}

func TestNewCatalogModelsKeepToolsUnlessExplicitlyUnsupported(test *testing.T) {
	for _, capability := range []struct {
		name, metadata string
		expectTools    bool
	}{
		{"unknown", "", true},
		{"unsupported", `,"tools":false`, false},
		{"supported", `,"tools":true`, true},
	} {
		test.Run(capability.name, func(test *testing.T) {
			requests := make(chan atom.Request, 1)
			server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/models" {
					fmt.Fprintf(responseWriter, `{"data":[{"id":"new-model"%s}]}`, capability.metadata)
					return
				}
				var payload struct {
					Model string            `json:"model"`
					Tools []json.RawMessage `json:"tools"`
				}
				testutil.RequireNoError(test, json.NewDecoder(request.Body).Decode(&payload))
				requests <- atom.Request{Model: payload.Model, Tools: make([]atom.ToolSpec, len(payload.Tools))}
				responseWriter.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(responseWriter, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			standardProvider := provider.New(atom.ProviderSpec{Name: "dynamic", APIURL: server.URL, ModelListURL: server.URL + "/models"})
			_, operationError := standardProvider.Refresh(context.Background())
			testutil.RequireNoError(test, operationError)
			testStack := newStack(test)
			testStack.harnessRuntime.Provider(standardProvider)
			testutil.RequireNoError(test, testStack.registry.Add(tools.NewSearch(testStack.registry)))
			session := testStack.instance(test, 2)
			session.Model = "dynamic/new-model"
			testutil.RequireNoError(test, testStack.database.Sessions().Save(context.Background(), session))
			testStack.user(test, session, "use tools if available")
			testutil.RequireNoError(test, testStack.loop.Run(context.Background(), session))
			payload := <-requests
			if (len(payload.Tools) > 0) != capability.expectTools || payload.Model != "new-model" {
				test.Fatalf("catalog capability was not honored on the provider wire: %+v", payload)
			}
		})
	}
}
