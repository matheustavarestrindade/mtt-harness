package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func (standardProvider *Standard) Stream(operationContext context.Context, request atom.Request) (harness.Stream, error) {
	messages, operationError := encodeMessages(request.Messages)
	if operationError != nil {
		return nil, operationError
	}
	payload := map[string]any{"model": request.Model, "messages": messages, "stream": true, "stream_options": map[string]any{"include_usage": true}}
	if len(request.Tools) > 0 {
		tools, operationError := encodeTools(request.Tools)
		if operationError != nil {
			return nil, operationError
		}
		payload["tools"] = tools
	}
	for key, value := range request.Params {
		switch key {
		case "model", "messages", "tools", "stream", "stream_options":
			return nil, fmt.Errorf("request parameter %q is reserved", key)
		}
		payload[key] = value
	}
	body, operationError := json.Marshal(payload)
	if operationError != nil {
		return nil, operationError
	}
	url := strings.TrimSuffix(standardProvider.providerSpec.APIURL, "/") + "/chat/completions"
	httpRequest, operationError := http.NewRequestWithContext(operationContext, http.MethodPost, url, bytes.NewReader(body))
	if operationError != nil {
		return nil, operationError
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	key, operationError := standardProvider.secret(operationContext)
	if operationError != nil {
		return nil, operationError
	}
	if key != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+key)
	}
	response, operationError := standardProvider.client.Do(httpRequest)
	if operationError != nil {
		return nil, operationError
	}
	if response.StatusCode != http.StatusOK {
		defer response.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(response.Body, 8192))
		return nil, fmt.Errorf("provider HTTP status %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
	}
	stream := &httpStream{parts: make(chan atom.ResponsePart, 64)}
	go parseSSE(operationContext, response.Body, stream)
	return stream, nil
}

func encodeTools(tools []atom.ToolSpec) ([]map[string]any, error) {
	var result []map[string]any
	for _, tool := range tools {
		var parameters any = map[string]any{"type": "object"}
		if len(tool.InputSchema.JSON) > 0 {
			if operationError := json.Unmarshal(tool.InputSchema.JSON, &parameters); operationError != nil {
				return nil, fmt.Errorf("schema for %s: %w", tool.Name, operationError)
			}
		}
		result = append(result, map[string]any{"type": "function", "function": map[string]any{"name": tool.Name, "description": tool.Description, "parameters": parameters}})
	}
	return result, nil
}
