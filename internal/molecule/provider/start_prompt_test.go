package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestChatGPTDoesNotAddHiddenStartupInstructions(test *testing.T) {
	requests := make(chan map[string]json.RawMessage, 1)
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		var payload map[string]json.RawMessage
		if operationError := json.NewDecoder(request.Body).Decode(&payload); operationError != nil {
			test.Error(operationError)
			http.Error(responseWriter, "invalid request", 400)
			return
		}
		requests <- payload
		responseWriter.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(responseWriter, "data: {\"type\":\"response.completed\",\"response\":{\"output\":[]}}\n\n")
	}))
	defer server.Close()
	standardProvider := New(atom.ProviderSpec{Name: "example-plan", APIURL: server.URL, Protocol: "responses", Authentication: "chatgpt"})
	stream, operationError := standardProvider.Stream(context.Background(), atom.Request{Model: "example-model", Messages: []atom.Message{{Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "hello"}}}}})
	testutil.RequireNoError(test, operationError)
	for {
		_, operationError := stream.Recv(context.Background())
		if errors.Is(operationError, io.EOF) {
			break
		}
		testutil.RequireNoError(test, operationError)
	}
	payload := <-requests
	if string(payload["instructions"]) != `""` {
		test.Fatalf("adapter injected instructions that were absent from the model request: %s", payload["instructions"])
	}
}
