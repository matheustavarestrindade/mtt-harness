package provider

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type httpStream struct {
	parts          chan atom.ResponsePart
	operationError error
}

func (responseStream *httpStream) Recv(operationContext context.Context) (atom.ResponsePart, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return atom.ResponsePart{}, operationError
	}
	select {
	case part, found := <-responseStream.parts:
		if !found {
			if responseStream.operationError != nil {
				return atom.ResponsePart{}, responseStream.operationError
			}
			return atom.ResponsePart{}, io.EOF
		}
		return part, nil
	case <-operationContext.Done():
		return atom.ResponsePart{}, operationContext.Err()
	}
}

type sseChunk struct {
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			Reasoning        string `json:"reasoning"`
			Audio            *struct {
				ID         string `json:"id"`
				Data       string `json:"data"`
				Transcript string `json:"transcript"`
			} `json:"audio"`
			Images []struct {
				ImageURL struct {
					URL string `json:"url"`
				} `json:"image_url"`
			} `json:"images"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *chatUsage `json:"usage"`
}

type toolAccumulator struct {
	identifier string
	name       string
	arguments  strings.Builder
	emitted    bool
}

func parseChatCompletionEvents(operationContext context.Context, body io.ReadCloser, stream *httpStream, providerName string) {
	defer body.Close()
	defer close(stream.parts)
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	accumulators := map[int]*toolAccumulator{}
	complete := false
	var terminationError error
	audioID := ""
	var reasoningText strings.Builder
	send := func(part atom.ResponsePart) bool {
		select {
		case stream.parts <- part:
			return true
		case <-operationContext.Done():
			stream.operationError = operationContext.Err()
			return false
		}
	}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			complete = true
			break
		}
		var chunk sseChunk
		if operationError := json.Unmarshal([]byte(data), &chunk); operationError != nil {
			stream.operationError = fmt.Errorf("invalid provider stream JSON: %w", operationError)
			return
		}
		if chunk.Error != nil {
			stream.operationError = fmt.Errorf("provider stream: %s", chunk.Error.Message)
			return
		}
		for _, choice := range chunk.Choices {
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				complete = true
				if reasonError := chatCompletionTermination(providerName, *choice.FinishReason); reasonError != nil {
					terminationError = reasonError
				}
			}
			reasoning := choice.Delta.ReasoningContent
			if reasoning == "" {
				reasoning = choice.Delta.Reasoning
			}
			if reasoning != "" {
				reasoningText.WriteString(reasoning)
				if !send(atom.ResponsePart{Reasoning: reasoning}) {
					return
				}
			}
			if choice.Delta.Content != "" {
				if !send(atom.ResponsePart{Text: choice.Delta.Content}) {
					return
				}
			}
			if audio := choice.Delta.Audio; audio != nil {
				if audio.ID != "" {
					audioID = audio.ID
				}
				if audio.Transcript != "" && !send(atom.ResponsePart{Text: audio.Transcript}) {
					return
				}
				if audio.Data != "" {
					data, operationError := base64.StdEncoding.DecodeString(audio.Data)
					if operationError != nil {
						stream.operationError = operationError
						return
					}
					if !send(atom.ResponsePart{Content: []atom.Content{{Type: atom.Audio, Data: data, MIME: "audio/wav", AudioID: audioID}}}) {
						return
					}
				}
			}
			for _, image := range choice.Delta.Images {
				if !send(atom.ResponsePart{Content: []atom.Content{{Type: atom.Image, URL: image.ImageURL.URL}}}) {
					return
				}
			}
			for _, call := range choice.Delta.ToolCalls {
				accumulator, found := accumulators[call.Index]
				if !found {
					accumulator = &toolAccumulator{}
					accumulators[call.Index] = accumulator
				}
				if call.ID != "" {
					accumulator.identifier = call.ID
				}
				if call.Function.Name != "" {
					accumulator.name = call.Function.Name
				}
				accumulator.arguments.WriteString(call.Function.Arguments)
				if accumulator.arguments.Len() > 4*1024*1024 {
					stream.operationError = fmt.Errorf("tool arguments exceed 4 MiB")
					return
				}
				if !accumulator.emitted && accumulator.identifier != "" && accumulator.name != "" && json.Valid([]byte(accumulator.arguments.String())) {
					accumulator.emitted = true
					index := call.Index
					if !send(atom.ResponsePart{ToolIndex: &index, ToolCall: &atom.ToolCall{
						ID:    accumulator.identifier,
						Name:  accumulator.name,
						Input: []byte(accumulator.arguments.String()),
					}}) {
						return
					}
				}
			}
		}
		if chunk.Usage != nil {
			usage, usageError := normalizeProviderUsage(chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens, chunk.Usage.PromptTokensDetails, chunk.Usage.CompletionTokensDetails, chunk.Usage.Cost)
			if usageError != nil {
				stream.operationError = usageError
				return
			}
			if !send(atom.ResponsePart{Usage: usage}) {
				return
			}
		}
	}
	if operationError := scanner.Err(); operationError != nil {
		stream.operationError = operationError
		return
	}
	if terminationError != nil {
		stream.operationError = terminationError
		return
	}
	if !complete {
		stream.operationError = io.ErrUnexpectedEOF
		return
	}
	for _, accumulator := range accumulators {
		if !accumulator.emitted {
			stream.operationError = fmt.Errorf("incomplete tool call %q", accumulator.name)
			return
		}
	}
	if reasoningText.Len() > 0 {
		send(atom.ResponsePart{ProviderState: &atom.ProviderState{Provider: providerName, ChatReasoning: reasoningText.String()}})
	}
}
