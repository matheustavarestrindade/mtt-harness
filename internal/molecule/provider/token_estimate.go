package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

// EstimateTokens follows the protocol's model-visible content. Four UTF-8 bytes
// per token is the coarse estimate used by the official Codex client. Known
// reasoning usage is preferable to ciphertext length; unknown encrypted items
// use Codex's base64/padding estimate, never one token per transport byte.
func (standardProvider *Standard) EstimateTokens(operationContext context.Context, request atom.Request) (harness.TokenEstimate, error) {
	protocol := standardProvider.providerSpec.Protocol
	if protocol == "" {
		protocol = "openai"
	}
	estimate := harness.TokenEstimate{InputTokens: 16, Identity: "standard-visible-v1:" + protocol + ":" + standardProvider.providerSpec.APIURL}
	if operationError := operationContext.Err(); operationError != nil {
		return estimate, operationError
	}
	definitions, operationError := encodeFunctionToolDefinitions(request.Tools)
	if operationError != nil {
		return estimate, operationError
	}
	if len(definitions) > 0 {
		encoded, operationError := json.Marshal(definitions)
		if operationError != nil {
			return estimate, operationError
		}
		estimate.InputTokens += estimateTextTokens(len(encoded))
	}
	for _, message := range request.Messages {
		if operationError := operationContext.Err(); operationError != nil {
			return estimate, operationError
		}
		estimate.InputTokens += 8
		for _, content := range message.Content {
			if content.Type == atom.Text {
				estimate.InputTokens += estimateTextTokens(len(content.Text))
				continue
			}
			// Media admission and encoding remain adapter-owned. Reserve media
			// space rather than tokenizing base64, file bytes, or a URL string.
			estimate.InputTokens += 16384
		}
		for _, call := range message.ToolCalls {
			arguments, operationError := call.ArgumentsText()
			if operationError != nil {
				return estimate, operationError
			}
			estimate.InputTokens += 8 + estimateTextTokens(len(call.Name)+len(arguments))
		}
		state := message.ProviderState
		if state == nil || state.Provider != standardProvider.Name() || message.Role != atom.RoleAssistant {
			continue
		}
		if protocol != "responses" {
			estimate.InputTokens += estimateTextTokens(len(state.ChatReasoning))
			continue
		}
		if len(state.Reasoning) == 0 {
			continue
		}
		if message.Usage != nil && message.Usage.Reasoning > 0 {
			estimate.InputTokens += message.Usage.Reasoning
			continue
		}
		for _, raw := range state.Reasoning {
			var item struct {
				Type             string `json:"type"`
				EncryptedContent string `json:"encrypted_content"`
			}
			if operationError := json.Unmarshal(raw, &item); operationError != nil {
				return estimate, fmt.Errorf("decode reasoning continuation for token estimate: %w", operationError)
			}
			if item.Type != "reasoning" {
				return estimate, fmt.Errorf("provider continuation must contain reasoning items only")
			}
			estimate.InputTokens += estimateTextTokens(max(0, len(item.EncryptedContent)*3/4-650))
		}
	}
	return estimate, nil
}

func estimateTextTokens(bytes int) int { return (bytes + 3) / 4 }
