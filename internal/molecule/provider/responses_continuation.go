package provider

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

const maximumContinuationItems = 16384

type responsesContinuation struct {
	responseID string
	properties [sha256.Size]byte
	items      [][sha256.Size]byte
}

func responsesPropertyFingerprint(payload map[string]any) ([sha256.Size]byte, error) {
	properties := make(map[string]any, len(payload))
	for name, value := range payload {
		if name != "input" && name != "stream" {
			properties[name] = value
		}
	}
	encoded, operationError := json.Marshal(properties)
	return sha256.Sum256(encoded), operationError
}

func responsesItemFingerprints(items []any) ([][sha256.Size]byte, error) {
	result := make([][sha256.Size]byte, 0, len(items))
	for _, item := range items {
		item = canonicalResponsesFunctionCall(item)
		encoded, operationError := json.Marshal(item)
		if operationError != nil {
			return nil, operationError
		}
		result = append(result, sha256.Sum256(encoded))
	}
	return result, nil
}

func canonicalResponsesFunctionCall(item any) any {
	fields, valid := item.(map[string]any)
	if !valid || fields["type"] != "function_call" {
		return item
	}
	arguments, valid := fields["arguments"].(string)
	if !valid || !json.Valid([]byte(arguments)) {
		return item
	}
	// JSONB normalizes tool-argument whitespace/key order on history reload.
	// Compare JSON values without changing the sent argument string or rounding
	// numbers through float64. All other item bytes remain significant.
	var value any
	decoder := json.NewDecoder(bytes.NewBufferString(arguments))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return item
	}
	encoded, operationError := json.Marshal(value)
	if operationError != nil {
		return item
	}
	canonicalFields := make(map[string]any, len(fields))
	for name, value := range fields {
		canonicalFields[name] = value
	}
	canonicalFields["arguments"] = string(encoded)
	return canonicalFields
}

func (baseline *responsesContinuation) incrementalInput(payload map[string]any) ([]any, bool) {
	if baseline == nil || baseline.responseID == "" {
		return nil, false
	}
	properties, operationError := responsesPropertyFingerprint(payload)
	if operationError != nil || properties != baseline.properties {
		return nil, false
	}
	input, valid := payload["input"].([]any)
	if !valid || len(input) < len(baseline.items) {
		return nil, false
	}
	fingerprints, operationError := responsesItemFingerprints(input[:len(baseline.items)])
	if operationError != nil {
		return nil, false
	}
	for index, fingerprint := range fingerprints {
		if fingerprint != baseline.items[index] {
			return nil, false
		}
	}
	return input[len(baseline.items):], true
}

// Compare the complete logical input with the actual representation this
// adapter replays. IDs/status on ordinary output messages are transport metadata;
// reasoning items, tool IDs, arguments, content, and ordering remain significant.
// Unknown server-side output kinds cannot establish a continuation baseline.
func captureResponsesContinuation(payload map[string]any, completed *responseEvent, providerName string) *responsesContinuation {
	if completed == nil || completed.Response.ID == "" || completed.continuationIncomplete {
		return nil
	}
	input, valid := payload["input"].([]any)
	if !valid || len(input) > maximumContinuationItems {
		return nil
	}
	message := atom.Message{Role: atom.RoleAssistant, ProviderState: &atom.ProviderState{Provider: providerName}}
	var text strings.Builder
	for _, raw := range completed.Response.Output {
		var item responseItem
		if json.Unmarshal(raw, &item) != nil {
			return nil
		}
		switch item.Type {
		case "reasoning":
			message.ProviderState.Reasoning = append(message.ProviderState.Reasoning, raw)
		case "function_call":
			if item.CallID == "" || item.Name == "" || item.Arguments == nil || !json.Valid([]byte(*item.Arguments)) {
				return nil
			}
			message.ToolCalls = append(message.ToolCalls, atom.ToolCall{ID: item.CallID, Name: item.Name, Input: json.RawMessage(*item.Arguments)})
		case "message":
			for _, content := range item.Content {
				switch content.Type {
				case "output_text":
					text.WriteString(content.Text)
				case "refusal":
					text.WriteString(content.Refusal)
				default:
					return nil
				}
			}
		default:
			return nil
		}
	}
	if text.Len() > 0 {
		message.Content = []atom.Content{{Type: atom.Text, Text: text.String()}}
	}
	output, _, operationError := encodeResponsesAPIInput([]atom.Message{message}, providerName)
	if operationError != nil || len(input)+len(output) > maximumContinuationItems {
		return nil
	}
	properties, operationError := responsesPropertyFingerprint(payload)
	if operationError != nil {
		return nil
	}
	inputFingerprints, operationError := responsesItemFingerprints(input)
	if operationError != nil {
		return nil
	}
	outputFingerprints, operationError := responsesItemFingerprints(output)
	if operationError != nil {
		return nil
	}
	return &responsesContinuation{responseID: completed.Response.ID, properties: properties, items: append(inputFingerprints, outputFingerprints...)}
}
