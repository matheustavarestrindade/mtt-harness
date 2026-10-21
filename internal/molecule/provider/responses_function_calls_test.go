package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func decodeFunctionCallFixture(events ...string) ([]atom.ResponsePart, error) {
	var body strings.Builder
	for _, event := range events {
		body.WriteString("data: " + event + "\n\n")
	}
	stream := &providerStream{parts: make(chan atom.ResponsePart, 64)}
	parseResponsesAPIEvents(context.Background(), io.NopCloser(strings.NewReader(body.String())), stream, "synthetic")
	var parts []atom.ResponsePart
	for part := range stream.parts {
		parts = append(parts, part)
	}
	return parts, stream.operationError
}

func TestResponsesIncompleteItemRetainsTerminalReasonUsageAndCompletedCalls(testingHandle *testing.T) {
	parts, operationError := decodeFunctionCallFixture(
		`{"type":"response.created","response":{"id":"response-one"}}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"item-one","call_id":"call-one","name":"inspect","arguments":"{}","status":"completed"}}`,
		`{"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","id":"item-two","call_id":"call-two","name":"write","arguments":"","status":"in_progress"}}`,
		`{"type":"response.function_call_arguments.delta","output_index":1,"item_id":"item-two","delta":"{\"content\":\"partial"}`,
		`{"type":"response.function_call_arguments.done","output_index":1,"item_id":"item-two","arguments":"{\"content\":\"partial"}`,
		`{"type":"response.output_item.done","output_index":1,"item":{"type":"function_call","id":"item-two","call_id":"call-two","name":"write","arguments":"{\"content\":\"partial","status":"incomplete"}}`,
		`{"type":"response.incomplete","response":{"id":"response-one","incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":80,"input_tokens_details":{"cached_tokens":20},"output_tokens":64,"output_tokens_details":{"reasoning_tokens":16}}}}`,
	)
	if operationError == nil || !strings.Contains(operationError.Error(), "max_output_tokens") || !strings.Contains(operationError.Error(), "call-two") {
		testingHandle.Fatalf("terminal diagnostic lost: %v", operationError)
	}
	var calls []atom.ToolCall
	var usage *atom.Usage
	for _, part := range parts {
		if part.ToolCall != nil {
			calls = append(calls, *part.ToolCall)
		}
		if part.Usage != nil {
			usage = part.Usage
		}
	}
	if len(calls) != 1 || calls[0].ID != "call-one" {
		testingHandle.Fatalf("partial call executed or valid call repeated: %+v", calls)
	}
	if usage == nil || usage.Input != 60 || usage.CacheRead != 20 || usage.Output != 64 || usage.Reasoning != 16 {
		testingHandle.Fatalf("terminal usage was discarded: %+v", usage)
	}
}

func TestResponsesAssemblesFunctionArgumentsAndSparseFinalItems(testingHandle *testing.T) {
	parts, operationError := decodeFunctionCallFixture(
		`{"type":"response.output_item.added","output_index":2,"item":{"type":"function_call","id":"item-a","call_id":"call-a","name":"inspect","arguments":"","status":"in_progress"}}`,
		`{"type":"response.function_call_arguments.delta","output_index":2,"item_id":"item-a","delta":"{\"number\":"}`,
		`{"type":"response.function_call_arguments.delta","output_index":2,"item_id":"item-a","delta":"9007199254740993}"}`,
		`{"type":"response.function_call_arguments.done","output_index":2,"item_id":"item-a","arguments":"{\"number\":9007199254740993}"}`,
		`{"type":"response.output_item.done","output_index":2,"item":{"type":"function_call","id":"item-a","status":"completed"}}`,
		`{"type":"response.completed","response":{"output":[{"type":"reasoning","id":"reason"},{"type":"message","content":[]},{"type":"function_call","id":"item-a","call_id":"call-a","name":"inspect","arguments":"{ \"number\": 9007199254740993 }","status":"completed"}]}}`,
	)
	if operationError != nil {
		testingHandle.Fatal(operationError)
	}
	calls := 0
	for _, part := range parts {
		if part.ToolCall == nil {
			continue
		}
		calls++
		if part.ToolIndex == nil || *part.ToolIndex != 2 || part.ToolCall.ID != "call-a" || !json.Valid(part.ToolCall.Input) || !strings.Contains(string(part.ToolCall.Input), "9007199254740993") {
			testingHandle.Fatalf("invalid assembled call: %+v", part)
		}
	}
	if calls != 1 {
		testingHandle.Fatalf("call count=%d", calls)
	}
}

func TestResponsesDefersInvalidItemUntilCompletedSnapshot(testingHandle *testing.T) {
	parts, operationError := decodeFunctionCallFixture(
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"item","call_id":"call","name":"inspect","arguments":"{"}}`,
		`{"type":"response.completed","response":{"output":[{"type":"function_call","id":"item","call_id":"call","name":"inspect","arguments":"{}","status":"completed"}],"usage":{"input_tokens":8,"output_tokens":4}}}`,
	)
	if operationError != nil {
		testingHandle.Fatal(operationError)
	}
	calls := 0
	for _, part := range parts {
		if part.ToolCall != nil {
			calls++
		}
	}
	if calls != 1 {
		testingHandle.Fatalf("call count=%d", calls)
	}
}

func TestResponsesNeverDeliversExplicitlyIncompleteOrMalformedCalls(testingHandle *testing.T) {
	for _, item := range []string{
		`{"type":"function_call","id":"item","call_id":"call","name":"inspect","arguments":"{}","status":"incomplete"}`,
		`{"type":"function_call","id":"item","call_id":"call","name":"inspect","arguments":"{","status":"completed"}`,
		`{"type":"function_call","id":"item","name":"inspect","arguments":"{}","status":"completed"}`,
	} {
		parts, operationError := decodeFunctionCallFixture(`{"type":"response.output_item.done","output_index":0,"item":`+item+`}`, `{"type":"response.completed","response":{"id":"invalid-call","output":[`+item+`],"usage":{"input_tokens":10,"output_tokens":5,"cost":0.25}}}`)
		if operationError == nil {
			testingHandle.Fatal("invalid call was accepted")
		}
		var usage *atom.Usage
		for _, part := range parts {
			if part.ToolCall != nil {
				testingHandle.Fatal("incomplete call was delivered")
			}
			if part.Usage != nil {
				usage = part.Usage
			}
		}
		if usage == nil || usage.Cost == nil || usage.Cost.Value != 0.25 {
			testingHandle.Fatalf("failure lost reported usage/cost: %+v", usage)
		}
	}
}

func TestResponsesInterleavesArgumentsByObservedIdentity(testingHandle *testing.T) {
	parts, operationError := decodeFunctionCallFixture(
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"a","call_id":"call-a","name":"one"}}`,
		`{"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","id":"b","call_id":"call-b","name":"two"}}`,
		`{"type":"response.function_call_arguments.delta","output_index":0,"item_id":"a","delta":"{\"x\":"}`,
		`{"type":"response.function_call_arguments.delta","output_index":1,"item_id":"b","delta":"{\"y\":2}"}`,
		`{"type":"response.output_item.done","output_index":1,"item":{"type":"function_call","id":"b","status":"completed"}}`,
		`{"type":"response.function_call_arguments.delta","output_index":0,"item_id":"a","delta":"1}"}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"a","status":"completed"}}`,
		`{"type":"response.completed","response":{"id":"interleaved","output":[]}}`,
	)
	if operationError != nil {
		testingHandle.Fatal(operationError)
	}
	var calls []atom.ResponsePart
	for _, part := range parts {
		if part.ToolCall != nil {
			calls = append(calls, part)
		}
	}
	if len(calls) != 2 || calls[0].ToolCall.ID != "call-b" || *calls[0].ToolIndex != 1 || string(calls[0].ToolCall.Input) != `{"y":2}` || calls[1].ToolCall.ID != "call-a" || *calls[1].ToolIndex != 0 || string(calls[1].ToolCall.Input) != `{"x":1}` {
		testingHandle.Fatalf("interleaved calls lost identity/order: %+v", calls)
	}
}

func TestResponsesRejectsChangedDeliveredArgumentsAndConflictingIdentity(testingHandle *testing.T) {
	parts, operationError := decodeFunctionCallFixture(
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"one","call_id":"call","name":"inspect","arguments":"{\"number\":9007199254740992}"}}`,
		`{"type":"response.completed","response":{"output":[{"type":"function_call","id":"one","call_id":"call","name":"inspect","arguments":"{\"number\":9007199254740993}"}]}}`,
	)
	if operationError == nil || !strings.Contains(operationError.Error(), "changed after delivery") {
		testingHandle.Fatalf("argument change was ignored: %v", operationError)
	}
	calls := 0
	for _, part := range parts {
		if part.ToolCall != nil {
			calls++
		}
	}
	if calls != 1 {
		testingHandle.Fatalf("changed call was delivered %d times", calls)
	}
	_, operationError = decodeFunctionCallFixture(
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"one","call_id":"a","name":"inspect"}}`,
		`{"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","id":"two","call_id":"b","name":"inspect"}}`,
		`{"type":"response.function_call_arguments.delta","output_index":1,"item_id":"one","delta":"{}"}`,
	)
	if operationError == nil || !strings.Contains(operationError.Error(), "different items") {
		testingHandle.Fatalf("conflicting identities were joined: %v", operationError)
	}
}

func TestResponsesSparseCompletionCannotBecomeAContinuationBaseline(testingHandle *testing.T) {
	stream := &providerStream{parts: make(chan atom.ResponsePart, 64)}
	decoder := newResponsesDecoder(context.Background(), stream, "synthetic")
	for _, event := range []string{
		`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"one","call_id":"a","name":"inspect"}}`,
		`{"type":"response.function_call_arguments.done","output_index":0,"item_id":"one","arguments":"{}"}`,
		`{"type":"response.completed","response":{"id":"response-one","output":[]}}`,
	} {
		if _, operationError := decoder.consumeEvent([]byte(event)); operationError != nil {
			testingHandle.Fatal(operationError)
		}
	}
	if decoder.completed == nil || captureResponsesContinuation(map[string]any{"input": []any{}}, decoder.completed, "synthetic") != nil {
		testingHandle.Fatal("sparse output enabled ambiguous continuation")
	}
}

func TestResponsesBoundsAccumulatedFunctionArguments(testingHandle *testing.T) {
	stream := &providerStream{parts: make(chan atom.ResponsePart, 64)}
	decoder := newResponsesDecoder(context.Background(), stream, "synthetic")
	index := 0
	if _, operationError := decoder.consumeFunctionArguments(responseEvent{Type: "response.function_call_arguments.delta", ItemID: "one", OutputIndex: &index, Delta: strings.Repeat("x", maximumResponsesEventBytes)}); operationError != nil {
		testingHandle.Fatal(operationError)
	}
	if _, operationError := decoder.consumeFunctionArguments(responseEvent{Type: "response.function_call_arguments.delta", ItemID: "one", OutputIndex: &index, Delta: "x"}); operationError == nil {
		testingHandle.Fatal("unbounded argument accumulation")
	}
	decoder = newResponsesDecoder(context.Background(), stream, "synthetic")
	for index := 0; index < maximumResponsesFunctionCalls; index++ {
		if _, operationError := decoder.resolveFunctionCall(fmt.Sprintf("item-%d", index), "", "", &index); operationError != nil {
			testingHandle.Fatal(operationError)
		}
	}
	if _, operationError := decoder.resolveFunctionCall("excess-item", "", "", nil); operationError == nil {
		testingHandle.Fatal("unbounded function call accumulation")
	}
}

func TestResponsesSparseTerminalArrayKeepsObservedCallIndex(testingHandle *testing.T) {
	stream := &providerStream{parts: make(chan atom.ResponsePart, 64)}
	decoder := newResponsesDecoder(context.Background(), stream, "synthetic")
	for _, event := range []string{
		`{"type":"response.output_item.done","output_index":2,"item":{"type":"function_call","id":"one","call_id":"a","name":"inspect","arguments":"{}"}}`,
		`{"type":"response.completed","response":{"id":"response-one","output":[{"type":"function_call","id":"one","call_id":"a","name":"inspect","arguments":"{}"}]}}`,
	} {
		if _, operationError := decoder.consumeEvent([]byte(event)); operationError != nil {
			testingHandle.Fatal(operationError)
		}
	}
	if decoder.completed == nil || !decoder.completed.continuationIncomplete {
		testingHandle.Fatal("sparse array was treated as a full output snapshot")
	}
	part := <-stream.parts
	if part.ToolIndex == nil || *part.ToolIndex != 2 || part.ToolCall == nil || part.ToolCall.ID != "a" {
		testingHandle.Fatalf("call index changed: %+v", part)
	}
	if len(stream.parts) != 0 {
		testingHandle.Fatal("sparse completion repeated a delivered call")
	}
}
