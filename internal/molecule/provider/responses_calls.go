package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

const maximumResponsesFunctionCalls = 1024

type responsesFunctionCall struct {
	itemID, callID, name string
	index                *int
	arguments            strings.Builder
	argumentsDone        bool
	itemDone             bool
	status               string
	emitted              bool
}

type responsesFunctionCalls struct {
	items         map[string]*responsesFunctionCall
	calls         map[string]*responsesFunctionCall
	indexes       map[int]*responsesFunctionCall
	ordered       []*responsesFunctionCall
	argumentBytes int
}

func newResponsesFunctionCalls() responsesFunctionCalls {
	return responsesFunctionCalls{items: map[string]*responsesFunctionCall{}, calls: map[string]*responsesFunctionCall{}, indexes: map[int]*responsesFunctionCall{}}
}

// Stream events can split identity and arguments. Join only observed identifiers;
// an output index never substitutes for a missing harness-visible call ID.
func (decoder *responsesDecoder) resolveFunctionCall(itemID, callID, name string, index *int) (*responsesFunctionCall, error) {
	if index != nil && *index < 0 {
		return nil, fmt.Errorf("Responses function call has negative output_index=%d", *index)
	}
	if len(itemID) > 4096 || len(callID) > 4096 || len(name) > 4096 {
		return nil, fmt.Errorf("Responses function call identifier exceeds 4096 bytes")
	}
	var selected *responsesFunctionCall
	candidates := []*responsesFunctionCall{decoder.functionCalls.items[itemID], decoder.functionCalls.calls[callID]}
	if index != nil {
		candidates = append(candidates, decoder.functionCalls.indexes[*index])
	}
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		if selected != nil && selected != candidate {
			return nil, fmt.Errorf("Responses function call identifiers refer to different items")
		}
		selected = candidate
	}
	if selected == nil {
		if itemID == "" && callID == "" && index == nil {
			return nil, fmt.Errorf("Responses function call has no item_id, call_id, or output_index")
		}
		if len(decoder.functionCalls.ordered) >= maximumResponsesFunctionCalls {
			return nil, fmt.Errorf("Responses response exceeds %d function calls", maximumResponsesFunctionCalls)
		}
		selected = &responsesFunctionCall{}
		decoder.functionCalls.ordered = append(decoder.functionCalls.ordered, selected)
	}
	if (itemID != "" && selected.itemID != "" && selected.itemID != itemID) ||
		(callID != "" && selected.callID != "" && selected.callID != callID) ||
		(name != "" && selected.name != "" && selected.name != name) ||
		(index != nil && selected.index != nil && *selected.index != *index) {
		return nil, fmt.Errorf("Responses function call identity changed; %s", selected.diagnostic())
	}
	if itemID != "" {
		selected.itemID = itemID
		decoder.functionCalls.items[itemID] = selected
	}
	if callID != "" {
		selected.callID = callID
		decoder.functionCalls.calls[callID] = selected
	}
	if name != "" {
		selected.name = name
	}
	if index != nil {
		position := *index
		selected.index = &position
		decoder.functionCalls.indexes[position] = selected
	}
	return selected, nil
}

func sameResponsesArguments(first, second string) bool {
	if first == second {
		return true
	}
	return reflect.DeepEqual(
		canonicalResponsesFunctionCall(map[string]any{"type": "function_call", "arguments": first}),
		canonicalResponsesFunctionCall(map[string]any{"type": "function_call", "arguments": second}),
	)
}

func (decoder *responsesDecoder) replaceFunctionArguments(call *responsesFunctionCall, arguments string) error {
	// Sparse final snapshots can leave this field empty after a full argument
	// stream. Empty text is not a replacement JSON value.
	if arguments == "" && call.arguments.Len() > 0 {
		return nil
	}
	if call.emitted {
		if !sameResponsesArguments(call.arguments.String(), arguments) {
			return fmt.Errorf("Responses function call arguments changed after delivery; %s", call.diagnostic())
		}
		return nil
	}
	if call.arguments.String() == arguments {
		return nil
	}
	retained := decoder.functionCalls.argumentBytes - call.arguments.Len() + len(arguments)
	if retained > maximumResponsesEventBytes {
		return fmt.Errorf("Responses function call arguments exceed 8 MiB")
	}
	call.arguments.Reset()
	call.arguments.WriteString(arguments)
	decoder.functionCalls.argumentBytes = retained
	return nil
}

func (decoder *responsesDecoder) consumeFunctionArguments(event responseEvent) (bool, error) {
	call, operationError := decoder.resolveFunctionCall(event.ItemID, "", "", event.OutputIndex)
	if operationError != nil {
		return false, operationError
	}
	if event.Type == "response.function_call_arguments.done" {
		if event.Arguments != nil {
			if operationError := decoder.replaceFunctionArguments(call, *event.Arguments); operationError != nil {
				return false, operationError
			}
		}
		call.argumentsDone = true
		return true, nil
	}
	if call.argumentsDone || call.itemDone || call.emitted {
		return false, fmt.Errorf("Responses function arguments delta follows completion; %s", call.diagnostic())
	}
	if decoder.functionCalls.argumentBytes+len(event.Delta) > maximumResponsesEventBytes {
		return false, fmt.Errorf("Responses function call arguments exceed 8 MiB")
	}
	call.arguments.WriteString(event.Delta)
	decoder.functionCalls.argumentBytes += len(event.Delta)
	return true, nil
}

func (decoder *responsesDecoder) consumeFunctionItem(item responseItem, index *int, done, allowDelivery bool) (bool, error) {
	call, operationError := decoder.resolveFunctionCall(item.ID, item.CallID, item.Name, index)
	if operationError != nil {
		return false, operationError
	}
	if item.Arguments != nil {
		if operationError := decoder.replaceFunctionArguments(call, *item.Arguments); operationError != nil {
			return false, operationError
		}
	}
	if item.Status != "" {
		if call.emitted && item.Status != "completed" {
			return false, fmt.Errorf("Responses function call status changed after delivery; %s", call.diagnostic())
		}
		call.status = item.Status
	} else if done && !call.itemDone && call.status == "in_progress" {
		// The done event itself closes legacy/compatible items that omit status.
		call.status = "completed"
	}
	call.itemDone = call.itemDone || done
	if !allowDelivery || !call.ready() {
		return true, nil
	}
	return decoder.deliverFunctionCall(call)
}

func (call *responsesFunctionCall) ready() bool {
	return call.finished() && json.Valid([]byte(call.arguments.String()))
}

func (call *responsesFunctionCall) finished() bool {
	return call.itemDone && (call.status == "" || call.status == "completed") && call.callID != "" && call.name != ""
}

func (decoder *responsesDecoder) deliverFunctionCall(call *responsesFunctionCall) (bool, error) {
	if call.emitted {
		return true, nil
	}
	toolCall := atom.ToolCall{ID: call.callID, Name: call.name, Input: json.RawMessage(call.arguments.String())}
	if !json.Valid(toolCall.Input) {
		// Only completeFunctionCalls reaches this path, after response.completed.
		// Preserve the original text as data and require a failed tool result.
		var raw json.RawMessage
		inputError := json.Unmarshal(toolCall.Input, &raw)
		encoded, operationError := json.Marshal(call.arguments.String())
		if operationError != nil {
			return false, operationError
		}
		toolCall.Input = encoded
		var syntaxError *json.SyntaxError
		if errors.As(inputError, &syntaxError) {
			toolCall.InputError = fmt.Sprintf("invalid JSON arguments at byte %d (%d bytes): %s", syntaxError.Offset, call.arguments.Len(), syntaxError.Error())
		} else {
			toolCall.InputError = fmt.Sprintf("invalid JSON arguments (%d bytes)", call.arguments.Len())
		}
	}
	call.emitted = true
	return decoder.send(atom.ResponsePart{ToolIndex: call.index, ToolCall: &toolCall}), nil
}

func (decoder *responsesDecoder) completeFunctionCalls() (bool, error) {
	for _, call := range decoder.functionCalls.ordered {
		if call.emitted {
			continue
		}
		// A successful response may omit redundant output snapshots. It can close
		// an explicitly finished arguments stream, but never an unfinished delta.
		if call.argumentsDone && !call.itemDone && (call.status == "" || call.status == "in_progress" || call.status == "completed") {
			call.itemDone = true
			call.status = "completed"
		}
		if !call.finished() {
			return false, fmt.Errorf("%s Responses response.completed has an incomplete function call; response_id=%q; %s", decoder.providerName, boundedProviderDetail(decoder.responseID), call.diagnostic())
		}
		if keepReading, operationError := decoder.deliverFunctionCall(call); operationError != nil || !keepReading {
			return keepReading, operationError
		}
	}
	return true, nil
}

func (call *responsesFunctionCall) diagnostic() string {
	index := "unknown"
	if call.index != nil {
		index = fmt.Sprint(*call.index)
	}
	return fmt.Sprintf("output_index=%s; item_id=%q; call_id=%q; tool=%q; item_status=%q; arguments_bytes=%d; arguments_valid_json=%t",
		index, boundedProviderDetail(call.itemID), boundedProviderDetail(call.callID), boundedProviderDetail(call.name), boundedProviderDetail(call.status), call.arguments.Len(), json.Valid([]byte(call.arguments.String())))
}

func (decoder *responsesDecoder) pendingFunctionDetails() string {
	for _, call := range decoder.functionCalls.ordered {
		if !call.emitted {
			state := "incomplete"
			if call.ready() {
				state = "undelivered"
			}
			return "; " + state + " Responses function call; " + call.diagnostic()
		}
	}
	return ""
}

// A sparse completion can be delivered from observed stream fields, but it is
// not a full logical-output snapshot for previous_response_id accounting.
func (decoder *responsesDecoder) functionSnapshotComplete(output []json.RawMessage) bool {
	seen := map[string]bool{}
	for index, raw := range output {
		var item responseItem
		if json.Unmarshal(raw, &item) != nil {
			return false
		}
		if item.Type != "function_call" {
			continue
		}
		call := decoder.functionCalls.calls[item.CallID]
		if call == nil || item.Name == "" || item.Arguments == nil || !json.Valid([]byte(*item.Arguments)) || (call.index != nil && *call.index != index) {
			return false
		}
		seen[item.CallID] = true
	}
	return len(seen) == len(decoder.functionCalls.ordered)
}
