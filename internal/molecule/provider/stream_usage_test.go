package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func readScriptedStream(test *testing.T, protocol, encoded string) (string, *atom.Usage, error) {
	test.Helper()
	stream := &httpStream{parts: make(chan atom.ResponsePart, 64)}
	if protocol == "responses" {
		go parseResponsesAPIEvents(context.Background(), io.NopCloser(strings.NewReader(encoded)), stream, "synthetic")
	} else {
		go parseChatCompletionEvents(context.Background(), io.NopCloser(strings.NewReader(encoded)), stream, "synthetic")
	}
	var text strings.Builder
	var usage *atom.Usage
	for {
		part, operationError := stream.Recv(context.Background())
		text.WriteString(part.Text)
		if part.Usage != nil {
			usage = part.Usage
		}
		if operationError != nil {
			return text.String(), usage, operationError
		}
	}
}

func TestChatTerminationPreservesPartialUsageAndRejectsInterruptedSuccess(test *testing.T) {
	for _, reason := range []string{"stop", "tool_calls", "function_call", "aborted", "insufficient_system_resource", "length", "content_filter", "future_failure"} {
		test.Run(reason, func(test *testing.T) {
			encoded := fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"content\":\"partial output\"},\"finish_reason\":%q}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":20,\"completion_tokens\":8,\"prompt_tokens_details\":{\"cached_tokens\":3,\"cache_write_tokens\":2},\"completion_tokens_details\":{\"reasoning_tokens\":4},\"cost\":0.95}}\n\ndata: [DONE]\n\n", reason)
			text, usage, operationError := readScriptedStream(test, "chat", encoded)
			normal := reason == "stop" || reason == "tool_calls" || reason == "function_call"
			if normal != errors.Is(operationError, io.EOF) {
				test.Fatalf("finish %q: %v", reason, operationError)
			}
			if !normal && !strings.Contains(operationError.Error(), reason) {
				test.Fatalf("missing failure reason: %v", operationError)
			}
			if text != "partial output" || usage == nil || usage.Input != 15 || usage.Output != 8 || usage.Reasoning != 4 || usage.Cost == nil || usage.Cost.Value != 0.95 || usage.Cost.Estimated {
				test.Fatalf("partial data/usage: %q %+v", text, usage)
			}
		})
	}
}

func TestProtocolReportedCostsRetainZeroUnknownAndCurrency(test *testing.T) {
	for _, protocol := range []string{"chat", "responses"} {
		for _, cost := range []string{"null", "0", "0.95", `{"currency":"EUR","value":0.3,"estimated":true}`} {
			test.Run(protocol+"/"+cost, func(test *testing.T) {
				encoded := fmt.Sprintf("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1,\"cost\":%s}}\n\ndata: [DONE]\n\n", cost)
				if protocol == "responses" {
					encoded = fmt.Sprintf("data: {\"type\":\"response.completed\",\"response\":{\"output\":[],\"usage\":{\"input_tokens\":2,\"output_tokens\":1,\"cost\":%s}}}\n\n", cost)
				}
				_, usage, operationError := readScriptedStream(test, protocol, encoded)
				if !errors.Is(operationError, io.EOF) || usage == nil {
					test.Fatalf("decode: %+v %v", usage, operationError)
				}
				switch cost {
				case "null":
					if usage.Cost != nil {
						test.Fatal("unknown cost became known")
					}
				case "0":
					if usage.Cost == nil || usage.Cost.Value != 0 || usage.Cost.Currency != "USD" {
						test.Fatal("reported zero was lost")
					}
				case "0.95":
					if usage.Cost == nil || usage.Cost.Value != 0.95 || usage.Cost.Estimated {
						test.Fatal("reported cost was changed")
					}
				default:
					if usage.Cost == nil || usage.Cost.Value != 0.3 || usage.Cost.Currency != "EUR" || !usage.Cost.Estimated {
						test.Fatal("structured cost was changed")
					}
				}
			})
		}
	}
}

func TestResponsesFailuresKeepBilledUsage(test *testing.T) {
	for _, eventType := range []string{"response.failed", "response.incomplete"} {
		encoded := fmt.Sprintf("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\ndata: {\"type\":%q,\"response\":{\"usage\":{\"input_tokens\":10,\"output_tokens\":2,\"cost\":0.2}}}\n\n", eventType)
		text, usage, operationError := readScriptedStream(test, "responses", encoded)
		if operationError == nil || errors.Is(operationError, io.EOF) || text != "partial" || usage == nil || usage.Cost == nil || usage.Cost.Value != 0.2 {
			test.Fatalf("failed response lost data: %q %+v %v", text, usage, operationError)
		}
	}
}

func TestReportedCostRejectsMalformedAmounts(test *testing.T) {
	for _, cost := range []string{`-1`, `"0.2"`, `{"currency":"USD"}`, `{"value":1}`, `{"currency":"USD","value":-0.1}`} {
		encoded := fmt.Sprintf("data: {\"choices\":[],\"usage\":{\"cost\":%s}}\n\ndata: [DONE]\n\n", cost)
		_, _, operationError := readScriptedStream(test, "chat", encoded)
		if operationError == nil || errors.Is(operationError, io.EOF) {
			test.Fatalf("invalid cost accepted: %s", cost)
		}
	}
}
