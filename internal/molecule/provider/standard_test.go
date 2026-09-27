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
)

func TestStandardStreamsParts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat/completions":
			if r.Header.Get("Authorization") != "Bearer key" {
				t.Errorf("the authorization header = %q", r.Header.Get("Authorization"))
			}
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if payload["model"] != "m1" {
				t.Errorf("the model = %v", payload["model"])
			}
			w.Header().Set("Content-Type", "text/event-stream")
			flusher := w.(http.Flusher)
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
				fmt.Fprintf(w, "%s\n\n", line)
				flusher.Flush()
			}
		case "/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"id": "m1"}, {"id": "m2"}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := New(atom.ProviderSpec{
		Name:         "test",
		APIURL:       server.URL,
		ModelListURL: server.URL + "/models",
		Secret:       "key",
	})
	stream, err := provider.Stream(context.Background(), atom.Request{
		Model: "m1",
		Messages: []atom.Message{{
			Role:    atom.RoleUser,
			Content: []atom.Content{{Type: atom.Text, Text: "hi"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var text string
	var calls []atom.ToolCall
	var usage *atom.Usage
	for {
		part, err := stream.Recv(context.Background())
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		text += part.Text
		if part.ToolCall != nil {
			calls = append(calls, *part.ToolCall)
		}
		if part.Usage != nil {
			usage = part.Usage
		}
	}
	if text != "Hello" {
		t.Fatalf("text = %q", text)
	}
	if len(calls) != 1 || calls[0].Name != "read" || string(calls[0].Input) != `{"path":"x"}` {
		t.Fatalf("calls = %+v", calls)
	}
	if usage == nil || usage.Input != 8 || usage.CacheRead != 2 || usage.Output != 5 {
		t.Fatalf("usage = %+v", usage)
	}

	priceFile := filepath.Join(t.TempDir(), "prices.json")
	if err := os.WriteFile(priceFile, []byte(`{"m1":{"currency":"USD","input":1.5,"output":3,"cache_read":0.5,"cache_write":0}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	provider.spec.PriceTableURL = priceFile
	models, err := provider.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 {
		t.Fatalf("models = %+v", models)
	}
	if models[0].Prices == nil || models[0].Prices.Input != 1.5 {
		t.Fatalf("prices = %+v", models[0].Prices)
	}
}
