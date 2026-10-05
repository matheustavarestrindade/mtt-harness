package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func (standardProvider *Standard) streamResponses(operationContext context.Context, request atom.Request) (harness.Stream, error) {
	input, instructions, operationError := encodeResponsesAPIInput(request.Messages, standardProvider.Name())
	if operationError != nil {
		return nil, operationError
	}
	payload := map[string]any{
		"model": request.Model, "input": input, "stream": true, "store": false,
		"include": []string{"reasoning.encrypted_content"}, "parallel_tool_calls": true,
	}
	// Keep the ChatGPT wire field present without adding provider-owned prompts.
	if instructions != "" || standardProvider.providerSpec.Authentication == "chatgpt" {
		payload["instructions"] = instructions
	}
	if len(request.Tools) > 0 {
		encoded, operationError := encodeFunctionToolDefinitions(request.Tools)
		if operationError != nil {
			return nil, operationError
		}
		tools := make([]map[string]any, 0, len(encoded))
		for _, tool := range encoded {
			function := tool["function"].(map[string]any)
			function["type"] = "function"
			function["strict"] = false
			tools = append(tools, function)
		}
		payload["tools"] = tools
	}
	for name, value := range request.Params {
		switch name {
		case "model", "input", "instructions", "tools", "stream", "store", "include", "previous_response_id", "conversation":
			return nil, fmt.Errorf("request parameter %q is reserved", name)
		}
		payload[name] = value
	}
	if operationError := standardProvider.applyReasoningParameters(payload, request); operationError != nil {
		return nil, operationError
	}
	body, operationError := json.Marshal(payload)
	if operationError != nil {
		return nil, operationError
	}
	httpRequest, operationError := http.NewRequestWithContext(operationContext, http.MethodPost, strings.TrimRight(standardProvider.providerSpec.APIURL, "/")+"/responses", bytes.NewReader(body))
	if operationError != nil {
		return nil, operationError
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "text/event-stream")
	if session, found := harness.SessionFrom(operationContext); found {
		httpRequest.Header.Set("session_id", string(session.ID))
	}
	if operationError := standardProvider.applyAuthenticationHeaders(httpRequest); operationError != nil {
		return nil, operationError
	}
	response, operationError := standardProvider.client.Do(httpRequest)
	if operationError != nil {
		return nil, operationError
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("%s Responses API returned HTTP %d; check provider access and the selected model", standardProvider.Name(), response.StatusCode)
	}
	stream := &httpStream{parts: make(chan atom.ResponsePart, 64)}
	go parseResponsesAPIEvents(operationContext, response.Body, stream, standardProvider.Name())
	return stream, nil
}
