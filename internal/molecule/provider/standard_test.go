package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestStandardStreamsParts(test *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/chat/completions":
			if request.Header.Get("Authorization") != "Bearer key" {
				test.Errorf("the authorization header = %q", request.Header.Get("Authorization"))
			}
			var payload map[string]any
			_ = json.NewDecoder(request.Body).Decode(&payload)
			if payload["model"] != "m1" {
				test.Errorf("the model = %v", payload["model"])
			}
			responseWriter.Header().Set("Content-Type", "text/event-stream")
			flusher := responseWriter.(http.Flusher)
			lines := []string{
				`data: {"choices":[{"delta":{"content":"Hel"}}]}`,
				`data: {"choices":[{"delta":{"content":"lo"}}]}`,
				`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"read","arguments":"{\"path\":"}}]}}]}`,
				`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"x\"}"}}]}}]}`,
				`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
				`data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":2}}}`,
				`data: [DONE]`,
			}
			for _, line := range lines {
				fmt.Fprintf(responseWriter, "%s\n\n", line)
				flusher.Flush()
			}
		case "/models":
			_ = json.NewEncoder(responseWriter).Encode(map[string]any{
				"data": []map[string]any{{"id": "m1"}, {"id": "m2"}},
			})
		default:
			http.NotFound(responseWriter, request)
		}
	}))
	defer server.Close()

	provider := New(atom.ProviderSpec{
		Name:         "test",
		APIURL:       server.URL,
		ModelListURL: server.URL + "/models",
	})
	provider.SetKeyResolver(func(operationContext context.Context, instanceID string, name string) (string, error) {
		return "key", nil
	})
	stream, operationError := provider.Stream(context.Background(), atom.Request{
		Model: "m1",
		Messages: []atom.Message{{
			Role:    atom.RoleUser,
			Content: []atom.Content{{Type: atom.Text, Text: "hi"}},
		}},
	})
	testutil.RequireNoError(test, operationError)

	var text string
	var calls []atom.ToolCall
	var usage *atom.Usage
	for {
		part, operationError := stream.Recv(context.Background())
		if operationError == io.EOF {
			break
		}
		testutil.RequireNoError(test, operationError)

		text += part.Text
		if part.ToolCall != nil {
			calls = append(calls, *part.ToolCall)
		}
		if part.Usage != nil {
			usage = part.Usage
		}
	}
	if text != "Hello" {
		test.Fatalf("text = %q", text)
	}
	if len(calls) != 1 || calls[0].Name != "read" || string(calls[0].Input) != `{"path":"x"}` {
		test.Fatalf("calls = %+v", calls)
	}
	if usage == nil || usage.Input != 8 || usage.CacheRead != 2 || usage.Output != 5 {
		test.Fatalf("usage = %+v", usage)
	}

	priceFile := filepath.Join(test.TempDir(), "prices.json")
	testutil.RequireNoError(test, os.WriteFile(priceFile, []byte(`{"m1":{"currency":"USD","input":1.5,"output":3,"cache_read":0.5,"cache_write":0}}`), 0o644))

	provider.providerSpec.PriceTableURL = priceFile
	models, operationError := provider.Refresh(context.Background())
	testutil.RequireNoError(test, operationError)

	if len(models) != 2 {
		test.Fatalf("models = %+v", models)
	}
	if models[0].Prices == nil || models[0].Prices.Input != 1.5 {
		test.Fatalf("prices = %+v", models[0].Prices)
	}
}
