package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestTokenEstimateUsesProtocolContentAndReportedReasoning(test *testing.T) {
	provider := New(atom.ProviderSpec{Name: "synthetic", Protocol: "responses"})
	message := atom.Message{Role: atom.RoleAssistant, Content: []atom.Content{{Type: atom.Text, Text: "answer"}},
		Reasoning: strings.Repeat("display summary", 10000), Usage: &atom.Usage{Output: 516, Reasoning: 516},
		ProviderState: &atom.ProviderState{Provider: "synthetic", Reasoning: []json.RawMessage{
			json.RawMessage(`{"type":"reasoning","encrypted_content":"` + strings.Repeat("A", 700000) + `"}`),
		}}}
	request := atom.Request{Model: "test-model", Messages: []atom.Message{message}}
	estimate, operationError := provider.EstimateTokens(context.Background(), request)
	testutil.RequireNoError(test, operationError)
	if estimate.InputTokens > 600 || estimate.InputTokens < 516 {
		test.Fatalf("transport bytes or display summaries became tokens: %+v", estimate)
	}
	request.Messages[0].ProviderState = &atom.ProviderState{Provider: "another-provider", Reasoning: message.ProviderState.Reasoning}
	foreign, operationError := provider.EstimateTokens(context.Background(), request)
	testutil.RequireNoError(test, operationError)
	if foreign.InputTokens >= 100 {
		test.Fatal("foreign provider state was charged to this request")
	}
	chat := New(atom.ProviderSpec{Name: "synthetic", Protocol: "openai"})
	request.Messages[0].ProviderState = &atom.ProviderState{Provider: "synthetic", Reasoning: message.ProviderState.Reasoning, ChatReasoning: strings.Repeat("text", 100)}
	chatEstimate, operationError := chat.EstimateTokens(context.Background(), request)
	testutil.RequireNoError(test, operationError)
	if chatEstimate.InputTokens >= 200 || chatEstimate.InputTokens < 100 {
		test.Fatal("chat counted unreplayed Responses continuation")
	}
}

func TestUnknownReasoningUsesCodexTransportEstimateAndRetainsMediaAllowance(test *testing.T) {
	provider := New(atom.ProviderSpec{Name: "synthetic", Protocol: "responses"})
	request := atom.Request{Messages: []atom.Message{{Role: atom.RoleAssistant, ProviderState: &atom.ProviderState{Provider: "synthetic", Reasoning: []json.RawMessage{json.RawMessage(`{"type":"reasoning","encrypted_content":"` + strings.Repeat("A", 12000) + `"}`)}}}}}
	estimate, operationError := provider.EstimateTokens(context.Background(), request)
	testutil.RequireNoError(test, operationError)
	if estimate.InputTokens != 16+8+(12000*3/4-650+3)/4 {
		test.Fatalf("unknown continuation estimate: %+v", estimate)
	}
	request.Messages[0].Content = []atom.Content{{Type: atom.Image, Data: []byte(strings.Repeat("A", 100000))}}
	media, operationError := provider.EstimateTokens(context.Background(), request)
	testutil.RequireNoError(test, operationError)
	if media.InputTokens-estimate.InputTokens != 16384 {
		test.Fatal("media bytes were tokenized instead of reserving media space")
	}
	operationContext, cancel := context.WithCancel(context.Background())
	cancel()
	if _, operationError := provider.EstimateTokens(operationContext, request); operationError == nil {
		test.Fatal("cancelled measurement continued")
	}
}
