package contextbuilder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

// CaptureReportedUsage binds a single provider report to the sent request. It
// changes only private accounting metadata; normal history writer fences decide
// whether this response can be persisted. Failed/cancelled responses do not call it.
func CaptureReportedUsage(operationContext context.Context, request atom.Request, provider harness.Provider, message *atom.Message) error {
	estimator, supported := provider.(harness.TokenEstimator)
	if !supported || message.Usage == nil {
		return nil
	}
	if _, exact := provider.(harness.TokenCounter); exact {
		return nil
	}
	usage := message.Usage
	if usage.Input < 0 || usage.CacheRead < 0 || usage.CacheWrite < 0 || usage.Output < 0 {
		return nil
	}
	estimate, operationError := estimator.EstimateTokens(operationContext, request)
	if operationError != nil {
		return operationError
	}
	if estimate.Identity == "" {
		return nil
	}
	prefix, overlays, operationError := requestFingerprints(operationContext, request.Messages)
	if operationError != nil {
		return operationError
	}
	tools, operationError := fingerprint(request.Tools)
	if operationError != nil {
		return operationError
	}
	parameters, operationError := fingerprint([]any{request.Params, request.ReasoningEffort})
	if operationError != nil {
		return operationError
	}
	response, operationError := messageFingerprint(*message)
	if operationError != nil {
		return operationError
	}
	state := atom.ProviderState{Provider: provider.Name()}
	if message.ProviderState != nil && message.ProviderState.Provider == provider.Name() {
		state = *message.ProviderState
	}
	state.ContextUsage = &atom.ContextUsage{Model: request.Model, Estimator: estimate.Identity,
		InputTokens: usage.Input + usage.CacheRead + usage.CacheWrite, OutputTokens: usage.Output,
		PrefixFingerprint: prefix, ResponseFingerprint: response, ToolsFingerprint: tools,
		ParametersFingerprint: parameters, OverlayFingerprints: overlays}
	message.ProviderState = &state
	return nil
}

func estimateFromReportedUsage(operationContext context.Context, request atom.Request, providerName string, estimator harness.TokenEstimator, identity string) (int, bool, error) {
	for position := len(request.Messages) - 1; position >= 0; position-- {
		message := request.Messages[position]
		if message.Role != atom.RoleAssistant || message.Ephemeral {
			continue
		}
		state := message.ProviderState
		if state == nil || state.Provider != providerName || state.ContextUsage == nil {
			return 0, false, nil
		}
		anchor := state.ContextUsage
		if anchor.Model != request.Model || anchor.Estimator != identity || anchor.InputTokens < 0 || anchor.OutputTokens < 0 {
			return 0, false, nil
		}
		response, operationError := messageFingerprint(message)
		if operationError != nil {
			return 0, false, operationError
		}
		if response != anchor.ResponseFingerprint {
			return 0, false, nil
		}
		prefix, _, operationError := requestFingerprints(operationContext, request.Messages[:position])
		if operationError != nil {
			return 0, false, operationError
		}
		parameters, operationError := fingerprint([]any{request.Params, request.ReasoningEffort})
		if operationError != nil {
			return 0, false, operationError
		}
		if prefix != anchor.PrefixFingerprint || parameters != anchor.ParametersFingerprint {
			return 0, false, nil
		}
		// Prefix removal, reordering, or editing invalidates the anchor. Changed
		// request-local overlays and tool definitions are charged in full; their
		// old cost is never subtracted from an aggregate provider report.
		extra := request
		extra.Messages = nil
		extra.Tools = nil
		overlays := map[string]int{}
		for _, hash := range anchor.OverlayFingerprints {
			overlays[hash]++
		}
		for index, candidate := range request.Messages {
			if index == position {
				continue
			}
			if !candidate.Ephemeral {
				if index > position {
					extra.Messages = append(extra.Messages, candidate)
				}
				continue
			}
			hash, operationError := messageFingerprint(candidate)
			if operationError != nil {
				return 0, false, operationError
			}
			if overlays[hash] > 0 {
				overlays[hash]--
				continue
			}
			extra.Messages = append(extra.Messages, candidate)
		}
		tools, operationError := fingerprint(request.Tools)
		if operationError != nil {
			return 0, false, operationError
		}
		if tools != anchor.ToolsFingerprint {
			extra.Tools = request.Tools
		}
		count := anchor.InputTokens + anchor.OutputTokens
		if len(extra.Messages) != 0 || len(extra.Tools) != 0 {
			addition, operationError := estimator.EstimateTokens(operationContext, extra)
			if operationError != nil {
				return 0, false, operationError
			}
			if addition.InputTokens < 0 {
				return 0, false, fmt.Errorf("provider token estimate is negative")
			}
			count += addition.InputTokens
		}
		return count, true, operationContext.Err()
	}
	return 0, false, operationContext.Err()
}

func requestFingerprints(operationContext context.Context, messages []atom.Message) (string, []string, error) {
	var stable, overlays []string
	for _, message := range messages {
		if operationError := operationContext.Err(); operationError != nil {
			return "", nil, operationError
		}
		hash, operationError := messageFingerprint(message)
		if operationError != nil {
			return "", nil, operationError
		}
		if message.Ephemeral {
			overlays = append(overlays, hash)
			continue
		}
		stable = append(stable, hash)
	}
	hash, operationError := fingerprint(stable)
	return hash, overlays, operationError
}

func messageFingerprint(message atom.Message) (string, error) {
	// ProviderState is excluded from public Message JSON, but continuation bytes
	// must participate in the anchor. Prior ContextUsage metadata must not.
	var continuation any
	if state := message.ProviderState; state != nil && (len(state.Reasoning) > 0 || state.ChatReasoning != "") {
		continuation = []any{state.Provider, state.Reasoning, state.ChatReasoning}
	}
	return fingerprint([]any{message.Role, message.Content, message.ToolCalls, message.ToolCallID, message.InContext, continuation})
}

func fingerprint(value any) (string, error) {
	encoded, operationError := json.Marshal(value)
	if operationError != nil {
		return "", operationError
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
