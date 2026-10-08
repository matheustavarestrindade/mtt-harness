package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestReminderKeepsItsPositionInActualResponsesPayload(test *testing.T) {
	requests := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		var payload map[string]any
		if json.NewDecoder(request.Body).Decode(&payload) != nil {
			http.Error(responseWriter, "bad JSON", 400)
			return
		}
		requests <- payload
		responseWriter.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(responseWriter, "data: {\"type\":\"response.completed\",\"response\":{\"output\":[]}}\n\n")
	}))
	defer server.Close()
	messages := []atom.Message{{Role: atom.RoleSystem, Content: []atom.Content{{Type: atom.Text, Text: "Stable startup instructions"}}}, {Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "Work on the task"}}}, {ID: "spaced_repetition:1", Role: atom.RoleSystem, Ephemeral: true, InContext: true, Content: []atom.Content{{Type: atom.Text, Text: "<spaced repetition>Follow the task.</spaced repetition>"}}}, {Role: atom.RoleAssistant, Content: []atom.Content{{Type: atom.Text, Text: "Task result"}}}}
	standard := New(atom.ProviderSpec{Name: "fixture", Protocol: "responses", APIURL: server.URL})
	stream, operationError := standard.Stream(context.Background(), atom.Request{Model: "synthetic-model", Messages: messages})
	testutil.RequireNoError(test, operationError)
	for {
		_, operationError := stream.Recv(context.Background())
		if errors.Is(operationError, io.EOF) {
			break
		}
		testutil.RequireNoError(test, operationError)
	}
	payload := <-requests
	if payload["instructions"] != "Stable startup instructions" {
		test.Fatalf("reminder changed cached instructions: %+v", payload)
	}
	input := payload["input"].([]any)
	if len(input) != 3 || input[1].(map[string]any)["role"] != "system" || input[1].(map[string]any)["content"] != messages[2].Content[0].Text {
		test.Fatalf("reminder lost its in-context position: %+v", input)
	}
	chat, operationError := encodeMessages(messages, "fixture")
	testutil.RequireNoError(test, operationError)
	if len(chat) != 4 || chat[2]["role"] != "system" || chat[2]["content"] != messages[2].Content[0].Text {
		test.Fatalf("chat reminder moved: %+v", chat)
	}
	encoded, operationError := json.Marshal(messages[2])
	testutil.RequireNoError(test, operationError)
	var public map[string]any
	testutil.RequireNoError(test, json.Unmarshal(encoded, &public))
	if public["InContext"] != nil || public["Ephemeral"] != nil {
		test.Fatal("private placement leaked into public message JSON")
	}
}
