package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type responseEvent struct {
	Type         string          `json:"type"`
	Delta        string          `json:"delta"`
	Text         string          `json:"text"`
	OutputIndex  int             `json:"output_index"`
	SummaryIndex int             `json:"summary_index"`
	ContentIndex int             `json:"content_index"`
	Item         json.RawMessage `json:"item"`
	Response     struct {
		Output []json.RawMessage `json:"output"`
		Usage  *struct {
			Input        int `json:"input_tokens"`
			Output       int `json:"output_tokens"`
			InputDetails struct {
				Cached     int `json:"cached_tokens"`
				CacheWrite int `json:"cache_write_tokens"`
			} `json:"input_tokens_details"`
			OutputDetails struct {
				Reasoning int `json:"reasoning_tokens"`
			} `json:"output_tokens_details"`
		} `json:"usage"`
	} `json:"response"`
}

type responseItem struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Summary   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"summary"`
	Content []struct {
		Type    string `json:"type"`
		Text    string `json:"text"`
		Refusal string `json:"refusal"`
	} `json:"content"`
}

func parseResponsesAPIEvents(operationContext context.Context, body io.ReadCloser, stream *httpStream, providerName string) {
	defer body.Close()
	defer close(stream.parts)
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	emittedCalls := map[string]bool{}
	streamedText := false
	state := &atom.ProviderState{Provider: providerName}
	send := func(part atom.ResponsePart) bool {
		select {
		case stream.parts <- part:
			return true
		case <-operationContext.Done():
			stream.operationError = operationContext.Err()
			return false
		}
	}
	reasoning := reasoningStream{send: send}
	consumeItem := func(raw json.RawMessage, index int, rememberReasoning bool) bool {
		var item responseItem
		if operationError := json.Unmarshal(raw, &item); operationError != nil {
			stream.operationError = fmt.Errorf("invalid Responses output item: %w", operationError)
			return false
		}
		switch item.Type {
		case "function_call":
			if emittedCalls[item.CallID] {
				return true
			}
			if item.CallID == "" || item.Name == "" || !json.Valid([]byte(item.Arguments)) {
				stream.operationError = fmt.Errorf("incomplete Responses function call")
				return false
			}
			emittedCalls[item.CallID] = true
			return send(atom.ResponsePart{ToolIndex: &index, ToolCall: &atom.ToolCall{ID: item.CallID, Name: item.Name, Input: json.RawMessage(item.Arguments)}})
		case "reasoning":
			for partIndex, part := range item.Summary {
				if part.Type == "summary_text" && !reasoning.completePart(reasoningPartKey(index, "summary", partIndex), part.Text) {
					return false
				}
			}
			for partIndex, part := range item.Content {
				if part.Type == "reasoning_text" && !reasoning.completePart(reasoningPartKey(index, "content", partIndex), part.Text) {
					return false
				}
			}
			if rememberReasoning {
				state.Reasoning = append(state.Reasoning, append(json.RawMessage(nil), raw...))
			}
		}
		return true
	}
	var data strings.Builder
	consumeEvent := func() bool {
		if data.Len() == 0 {
			return true
		}
		encoded := data.String()
		data.Reset()
		var event responseEvent
		if operationError := json.Unmarshal([]byte(encoded), &event); operationError != nil {
			stream.operationError = fmt.Errorf("invalid Responses stream JSON: %w", operationError)
			return false
		}
		switch event.Type {
		case "response.reasoning_summary_text.delta":
			return reasoning.appendDelta(reasoningPartKey(event.OutputIndex, "summary", event.SummaryIndex), event.Delta)
		case "response.reasoning_text.delta":
			return reasoning.appendDelta(reasoningPartKey(event.OutputIndex, "content", event.ContentIndex), event.Delta)
		case "response.reasoning_summary_text.done":
			return reasoning.completePart(reasoningPartKey(event.OutputIndex, "summary", event.SummaryIndex), event.Text)
		case "response.reasoning_text.done":
			return reasoning.completePart(reasoningPartKey(event.OutputIndex, "content", event.ContentIndex), event.Text)
		case "response.output_text.delta", "response.refusal.delta":
			streamedText = true
			return send(atom.ResponsePart{Text: event.Delta})
		case "response.output_item.done":
			return consumeItem(event.Item, event.OutputIndex, false)
		case "response.completed":
			for index, raw := range event.Response.Output {
				if !consumeItem(raw, index, true) {
					return false
				}
				if !streamedText {
					var item responseItem
					if operationError := json.Unmarshal(raw, &item); operationError != nil {
						stream.operationError = operationError
						return false
					}
					for _, content := range item.Content {
						if content.Type == "output_text" && !send(atom.ResponsePart{Text: content.Text}) {
							return false
						}
						if content.Type == "refusal" && !send(atom.ResponsePart{Text: content.Refusal}) {
							return false
						}
					}
				}
			}
			if len(state.Reasoning) > 0 && !send(atom.ResponsePart{ProviderState: state}) {
				return false
			}
			if usage := event.Response.Usage; usage != nil {
				if usage.Input < 0 || usage.Output < 0 || usage.InputDetails.Cached < 0 || usage.InputDetails.CacheWrite < 0 || usage.InputDetails.Cached+usage.InputDetails.CacheWrite > usage.Input || usage.OutputDetails.Reasoning < 0 || usage.OutputDetails.Reasoning > usage.Output {
					stream.operationError = fmt.Errorf("invalid Responses token usage")
					return false
				}
				if !send(atom.ResponsePart{Usage: &atom.Usage{Input: usage.Input - usage.InputDetails.Cached - usage.InputDetails.CacheWrite, CacheRead: usage.InputDetails.Cached, CacheWrite: usage.InputDetails.CacheWrite, Output: usage.Output, Reasoning: usage.OutputDetails.Reasoning}}) {
					return false
				}
			}
			return false
		case "response.failed", "response.incomplete", "error":
			stream.operationError = fmt.Errorf("%s stream ended with %s", providerName, event.Type)
			return false
		}
		return true
	}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			if !consumeEvent() {
				return
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			if data.Len() > 8*1024*1024 {
				stream.operationError = fmt.Errorf("Responses event exceeds 8 MiB")
				return
			}
		}
	}
	if operationError := scanner.Err(); operationError != nil {
		stream.operationError = operationError
		return
	}
	if data.Len() > 0 && !consumeEvent() {
		return
	}
	stream.operationError = io.ErrUnexpectedEOF
}
