package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestReasoningEffortAndSummaryReachProviderPayloads(test *testing.T) {
	for _, protocol := range []string{"responses", "chat_completions"} {
		test.Run(protocol, func(test *testing.T) {
			requests := make(chan map[string]any, 3)
			server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				var payload map[string]any
				testutil.RequireNoError(test, json.NewDecoder(request.Body).Decode(&payload))
				requests <- payload
				if protocol == "responses" {
					fmt.Fprint(responseWriter, "data: {\"type\":\"response.completed\",\"response\":{\"output\":[]}}\n\n")
					return
				}
				fmt.Fprint(responseWriter, "data: [DONE]\n\n")
			}))
			defer server.Close()
			standardProvider := New(atom.ProviderSpec{Name: "fixture", Protocol: protocol, APIURL: server.URL})
			standardProvider.SetModels([]atom.ModelInfo{{ID: "reasoning-model", Reasoning: true, ReasoningEfforts: []string{"none", "low", "high"}, DefaultReasoningEffort: "low", ReasoningSummary: "auto"}})
			for _, effort := range []string{"high", "none", ""} {
				stream, operationError := standardProvider.Stream(context.Background(), atom.Request{Model: "reasoning-model", ReasoningEffort: effort})
				testutil.RequireNoError(test, operationError)
				_, operationError = stream.Recv(context.Background())
				if operationError != io.EOF {
					test.Fatalf("unexpected stream completion: %v", operationError)
				}
				payload := <-requests
				expected := effort
				if expected == "" {
					expected = "low"
				}
				if protocol == "chat_completions" {
					if payload["reasoning_effort"] != expected {
						test.Fatalf("effort lost: %+v", payload)
					}
					continue
				}
				reasoning := payload["reasoning"].(map[string]any)
				if reasoning["effort"] != expected || effort != "none" && reasoning["summary"] != "auto" || effort == "none" && reasoning["summary"] != nil {
					test.Fatalf("wrong reasoning payload: %+v", payload)
				}
			}
			if _, operationError := standardProvider.Stream(context.Background(), atom.Request{Model: "reasoning-model", ReasoningEffort: "unsupported"}); operationError == nil {
				test.Fatal("unsupported effort reached the provider")
			}
		})
	}
}

func TestResponsesReasoningIsPublicTextOnlyAndNotDuplicated(test *testing.T) {
	item := `{"type":"reasoning","encrypted_content":"private-ciphertext","summary":[{"type":"summary_text","text":"First step"},{"type":"summary_text","text":"Second step"}]}`
	body := "data: {\"type\":\"response.reasoning_summary_text.delta\",\"output_index\":0,\"summary_index\":0,\"delta\":\"First \"}\n\n" +
		"data: {\"type\":\"response.reasoning_summary_text.delta\",\"output_index\":0,\"summary_index\":0,\"delta\":\"step\"}\n\n" +
		"data: {\"type\":\"response.reasoning_summary_text.done\",\"output_index\":0,\"summary_index\":0,\"text\":\"First step\"}\n\n" +
		"data: {\"type\":\"response.output_item.done\",\"output_index\":0,\"item\":" + item + "}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"output\":[" + item + "],\"usage\":{\"input_tokens\":20,\"output_tokens\":10,\"output_tokens_details\":{\"reasoning_tokens\":8}}}}\n\n"
	stream := &httpStream{parts: make(chan atom.ResponsePart, 16)}
	go parseResponsesAPIEvents(context.Background(), io.NopCloser(strings.NewReader(body)), stream, "fixture")
	var reasoning strings.Builder
	var state *atom.ProviderState
	for {
		part, operationError := stream.Recv(context.Background())
		if operationError == io.EOF {
			break
		}
		testutil.RequireNoError(test, operationError)
		reasoning.WriteString(part.Reasoning)
		if part.ProviderState != nil {
			state = part.ProviderState
		}
		if part.Usage != nil && (part.Usage.Output != 10 || part.Usage.Reasoning != 8) {
			test.Fatal("reasoning was added to output tokens")
		}
	}
	if reasoning.String() != "First step\n\nSecond step" || state == nil || len(state.Reasoning) != 1 {
		test.Fatalf("reasoning loss or duplication: %q, %+v", reasoning.String(), state)
	}
	public, operationError := json.Marshal(atom.Message{Reasoning: reasoning.String(), ProviderState: state})
	testutil.RequireNoError(test, operationError)
	if strings.Contains(string(public), "private-ciphertext") || !strings.Contains(string(state.Reasoning[0]), "private-ciphertext") {
		test.Fatal("public text and private continuation data were not kept separate")
	}
}

func TestNativeReasoningTextSurvivesBothProtocols(test *testing.T) {
	for _, protocol := range []string{"responses", "chat_completions"} {
		test.Run(protocol, func(test *testing.T) {
			stream := &httpStream{parts: make(chan atom.ResponsePart, 16)}
			body := "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"Visible thinking\"}}]}\n\ndata: [DONE]\n\n"
			if protocol == "responses" {
				body = "data: {\"type\":\"response.reasoning_text.delta\",\"delta\":\"Visible thinking\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"reasoning\",\"content\":[{\"type\":\"reasoning_text\",\"text\":\"Visible thinking\"}]}]}}\n\n"
				go parseResponsesAPIEvents(context.Background(), io.NopCloser(strings.NewReader(body)), stream, "fixture")
			} else {
				go parseChatCompletionEvents(context.Background(), io.NopCloser(strings.NewReader(body)), stream, "fixture")
			}
			var reasoning string
			var state *atom.ProviderState
			for {
				part, operationError := stream.Recv(context.Background())
				if operationError == io.EOF {
					break
				}
				testutil.RequireNoError(test, operationError)
				reasoning += part.Reasoning
				if part.ProviderState != nil {
					state = part.ProviderState
				}
			}
			if reasoning != "Visible thinking" || state == nil {
				test.Fatalf("native reasoning missing: %q", reasoning)
			}
			if protocol != "chat_completions" {
				return
			}
			messages := []atom.Message{{Role: atom.RoleAssistant, Reasoning: reasoning, ProviderState: state}}
			encoded, operationError := encodeMessages(messages, "fixture")
			testutil.RequireNoError(test, operationError)
			if encoded[0]["reasoning_content"] != reasoning {
				test.Fatal("chat tool continuation lost reasoning")
			}
			encoded, operationError = encodeMessages(messages, "other-provider")
			testutil.RequireNoError(test, operationError)
			if encoded[0]["reasoning_content"] != nil {
				test.Fatal("reasoning continuation crossed providers")
			}
		})
	}
}
