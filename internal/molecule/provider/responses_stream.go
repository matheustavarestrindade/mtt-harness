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

const maximumResponsesEventBytes = 8 * 1024 * 1024

type responseEvent struct {
	Type         string                `json:"type"`
	Delta        string                `json:"delta"`
	Text         string                `json:"text"`
	ResponseID   string                `json:"response_id"`
	OutputIndex  int                   `json:"output_index"`
	SummaryIndex int                   `json:"summary_index"`
	ContentIndex int                   `json:"content_index"`
	Item         json.RawMessage       `json:"item"`
	Status       int                   `json:"status"`
	Error        *responsesErrorDetail `json:"error"`
	Response     struct {
		ID                string                `json:"id"`
		Output            []json.RawMessage     `json:"output"`
		Usage             *responsesUsage       `json:"usage"`
		Error             *responsesErrorDetail `json:"error"`
		IncompleteDetails struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
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

// responsesDecoder is shared by HTTP/SSE and WebSocket framing. Only a
// response.completed event publishes a reusable continuation baseline.
type responsesDecoder struct {
	providerName string
	send         func(atom.ResponsePart) bool
	reasoning    reasoningStream
	emittedCalls map[string]bool
	streamedText bool
	state        *atom.ProviderState
	completed    *responseEvent
	responseID   string
}

func newResponsesDecoder(operationContext context.Context, stream *providerStream, providerName string) *responsesDecoder {
	decoder := &responsesDecoder{providerName: providerName, emittedCalls: map[string]bool{}, state: &atom.ProviderState{Provider: providerName}}
	decoder.send = func(part atom.ResponsePart) bool {
		select {
		case stream.parts <- part:
			return true
		case <-operationContext.Done():
			stream.operationError = operationContext.Err()
			return false
		}
	}
	decoder.reasoning = reasoningStream{send: decoder.send}
	return decoder
}

func (decoder *responsesDecoder) consumeOutputItem(raw json.RawMessage, index int, rememberReasoning bool) (bool, error) {
	var item responseItem
	if operationError := json.Unmarshal(raw, &item); operationError != nil {
		return false, fmt.Errorf("invalid Responses output item: %w", operationError)
	}
	switch item.Type {
	case "function_call":
		if decoder.emittedCalls[item.CallID] {
			return true, nil
		}
		if item.CallID == "" || item.Name == "" || !json.Valid([]byte(item.Arguments)) {
			return false, fmt.Errorf("incomplete Responses function call")
		}
		decoder.emittedCalls[item.CallID] = true
		return decoder.send(atom.ResponsePart{ToolIndex: &index, ToolCall: &atom.ToolCall{ID: item.CallID, Name: item.Name, Input: json.RawMessage(item.Arguments)}}), nil
	case "reasoning":
		for partIndex, part := range item.Summary {
			if part.Type == "summary_text" && !decoder.reasoning.completePart(reasoningPartKey(index, "summary", partIndex), part.Text) {
				return false, nil
			}
		}
		for partIndex, part := range item.Content {
			if part.Type == "reasoning_text" && !decoder.reasoning.completePart(reasoningPartKey(index, "content", partIndex), part.Text) {
				return false, nil
			}
		}
		if rememberReasoning {
			decoder.state.Reasoning = append(decoder.state.Reasoning, append(json.RawMessage(nil), raw...))
		}
	}
	return true, nil
}

func (decoder *responsesDecoder) consumeEvent(encoded []byte) (bool, error) {
	var event responseEvent
	if operationError := json.Unmarshal(encoded, &event); operationError != nil {
		return false, fmt.Errorf("invalid Responses stream JSON: %w", operationError)
	}
	if event.Type == "response.created" {
		decoder.responseID = event.Response.ID
	}
	if event.ResponseID != "" && decoder.responseID != "" && event.ResponseID != decoder.responseID {
		return false, fmt.Errorf("Responses event belongs to a different response")
	}
	switch event.Type {
	case "response.completed", "response.failed", "response.incomplete":
		if decoder.responseID != "" && event.Response.ID != "" && decoder.responseID != event.Response.ID {
			return false, fmt.Errorf("Responses completion belongs to a different response")
		}
		if reported := event.Response.Usage; reported != nil {
			usage, operationError := normalizeProviderUsage(reported.Input, reported.Output, reported.InputDetails, reported.OutputDetails, reported.Cost)
			if operationError != nil {
				return false, operationError
			}
			if !decoder.send(atom.ResponsePart{Usage: usage}) {
				return false, nil
			}
		}
	}
	switch event.Type {
	case "response.reasoning_summary_text.delta":
		return decoder.reasoning.appendDelta(reasoningPartKey(event.OutputIndex, "summary", event.SummaryIndex), event.Delta), nil
	case "response.reasoning_text.delta":
		return decoder.reasoning.appendDelta(reasoningPartKey(event.OutputIndex, "content", event.ContentIndex), event.Delta), nil
	case "response.reasoning_summary_text.done":
		return decoder.reasoning.completePart(reasoningPartKey(event.OutputIndex, "summary", event.SummaryIndex), event.Text), nil
	case "response.reasoning_text.done":
		return decoder.reasoning.completePart(reasoningPartKey(event.OutputIndex, "content", event.ContentIndex), event.Text), nil
	case "response.output_text.delta", "response.refusal.delta":
		decoder.streamedText = true
		return decoder.send(atom.ResponsePart{Text: event.Delta}), nil
	case "response.output_item.done":
		return decoder.consumeOutputItem(event.Item, event.OutputIndex, false)
	case "response.completed":
		for index, raw := range event.Response.Output {
			keepReading, operationError := decoder.consumeOutputItem(raw, index, true)
			if operationError != nil || !keepReading {
				return false, operationError
			}
			if !decoder.streamedText {
				var item responseItem
				if operationError := json.Unmarshal(raw, &item); operationError != nil {
					return false, operationError
				}
				for _, content := range item.Content {
					if content.Type == "output_text" && !decoder.send(atom.ResponsePart{Text: content.Text}) {
						return false, nil
					}
					if content.Type == "refusal" && !decoder.send(atom.ResponsePart{Text: content.Refusal}) {
						return false, nil
					}
				}
			}
		}
		if len(decoder.state.Reasoning) > 0 && !decoder.send(atom.ResponsePart{ProviderState: decoder.state}) {
			return false, nil
		}
		decoder.completed = &event
		return false, nil
	case "response.failed", "response.incomplete", "error":
		detail := event.Error
		if detail == nil {
			detail = event.Response.Error
		}
		message := fmt.Sprintf("%s stream ended with %s", decoder.providerName, event.Type)
		if event.Status > 0 {
			message += fmt.Sprintf("; HTTP %d", event.Status)
		}
		if detail != nil {
			if code := boundedProviderDetail(detail.code()); code != "" && code != "null" {
				message += "; code=" + code
			}
			if detail.Message != "" {
				message += ": " + boundedProviderDetail(detail.Message)
			}
		}
		if reason := event.Response.IncompleteDetails.Reason; reason != "" {
			message += "; reason=" + boundedProviderDetail(reason)
		}
		return false, fmt.Errorf("%s", message)
	}
	return true, nil
}

func parseResponsesAPIEvents(operationContext context.Context, body io.ReadCloser, stream *providerStream, providerName string) {
	defer body.Close()
	defer stream.finishResponse()
	decoder := newResponsesDecoder(operationContext, stream, providerName)
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), maximumResponsesEventBytes)
	var data strings.Builder
	consume := func() bool {
		if data.Len() == 0 {
			return true
		}
		keepReading, operationError := decoder.consumeEvent([]byte(data.String()))
		data.Reset()
		if operationError != nil {
			stream.operationError = operationError
		}
		return keepReading && operationError == nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			if !consume() {
				return
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			if data.Len() > maximumResponsesEventBytes {
				stream.operationError = fmt.Errorf("Responses event exceeds 8 MiB")
				return
			}
		}
	}
	if operationError := scanner.Err(); operationError != nil {
		stream.operationError = operationError
		return
	}
	if data.Len() > 0 && !consume() {
		return
	}
	stream.operationError = io.ErrUnexpectedEOF
}
