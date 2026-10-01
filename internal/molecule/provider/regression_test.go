package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestConfiguredPricesOverrideCachedPrices(test *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		io.WriteString(responseWriter, `{"data":[{"id":"model"}]}`)
	}))
	defer server.Close()
	provider := New(atom.ProviderSpec{Name: "test", ModelListURL: server.URL})
	provider.SetPrices(map[string]atom.Prices{"model": {Currency: "USD", Input: 2}})
	provider.SetModels([]atom.ModelInfo{{ID: "model", Prices: &atom.Prices{Currency: "USD", Input: 1}}})
	models, operationError := provider.Refresh(context.Background())
	testutil.RequireNoError(test, operationError)
	if models[0].Prices.Input != 2 {
		test.Fatalf("stale price survived refresh: %+v", models[0].Prices)
	}
}

func TestMediaEncodingPreservesPayloadAndToolPairs(test *testing.T) {
	contents := []atom.Content{{Type: atom.Text, Text: "text"}, {Type: atom.Image, Data: []byte("image"), MIME: "image/png"}, {Type: atom.Audio, Data: []byte("audio"), MIME: "audio/wav"}, {Type: atom.File, Data: []byte("file"), MIME: "application/pdf", Filename: "report.pdf"}}
	encoded, operationError := encodeChatMessageContent(contents)
	testutil.RequireNoError(test, operationError)
	parts := encoded.([]map[string]any)
	for index, kind := range []string{"text", "image_url", "input_audio", "file"} {
		if parts[index]["type"] != kind {
			test.Fatalf("media lost: %+v", parts)
		}
	}
	messages, operationError := encodeMessages([]atom.Message{
		{Role: atom.RoleAssistant, ToolCalls: []atom.ToolCall{{ID: "one", Name: "image", Input: []byte(`{}`)}, {ID: "two", Name: "text", Input: []byte(`{}`)}}},
		{Role: atom.RoleTool, ToolCallID: "one", Content: contents[1:2]},
		{Role: atom.RoleTool, ToolCallID: "two", Content: contents[:1]},
	})
	testutil.RequireNoError(test, operationError)
	if len(messages) != 4 || messages[1]["role"] != "tool" || messages[2]["role"] != "tool" || messages[3]["role"] != "user" {
		test.Fatalf("media interrupted paired tool replies: %+v", messages)
	}
	serialized, operationError := json.Marshal(messages)
	testutil.RequireNoError(test, operationError)
	if !strings.Contains(string(serialized), "aW1hZ2U=") {
		test.Fatal("image bytes disappeared")
	}
}

func TestMalformedOrTruncatedProviderStreamFails(test *testing.T) {
	for _, body := range []string{`data: {bad json}` + "\n", `data: {"choices":[{"delta":{"content":"partial"}}]}` + "\n"} {
		stream := &httpStream{parts: make(chan atom.ResponsePart, 4)}
		go parseChatCompletionEvents(context.Background(), io.NopCloser(strings.NewReader(body)), stream)
		for {
			_, operationError := stream.Recv(context.Background())
			if operationError != nil {
				if operationError == io.EOF {
					test.Fatal("truncated response reported success")
				}
				break
			}
		}
	}
}
