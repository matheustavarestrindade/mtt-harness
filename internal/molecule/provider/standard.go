package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type KeyResolver func(ctx context.Context, instanceID string, provider string) (string, error)

type Standard struct {
	spec         atom.ProviderSpec
	client       *http.Client
	mu           sync.RWMutex
	models       []atom.ModelInfo
	inlinePrices map[string]atom.Prices
	key          KeyResolver
}

func New(spec atom.ProviderSpec) *Standard {
	return &Standard{
		spec:   spec,
		client: &http.Client{Timeout: 0},
	}
}

func (s *Standard) Name() string {
	return s.spec.Name
}

func (s *Standard) Models() []atom.ModelInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]atom.ModelInfo(nil), s.models...)
}

func (s *Standard) SetModels(models []atom.ModelInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.models = append([]atom.ModelInfo(nil), models...)
}

func (s *Standard) SetKeyResolver(resolver KeyResolver) {
	s.key = resolver
}

func (s *Standard) secret(ctx context.Context) string {
	if s.key == nil {
		return ""
	}
	instanceID := ""
	if session, ok := harness.SessionFrom(ctx); ok {
		instanceID = session.InstanceID
	}
	value, err := s.key(ctx, instanceID, s.spec.Name)
	if err != nil {
		return ""
	}
	return value
}

func (s *Standard) SetPrices(prices map[string]atom.Prices) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inlinePrices = map[string]atom.Prices{}
	for id, price := range prices {
		s.inlinePrices[id] = price
	}
	for index := range s.models {
		if s.models[index].Prices != nil {
			continue
		}
		if price, ok := s.inlinePrices[s.models[index].ID]; ok {
			value := price
			s.models[index].Prices = &value
		}
	}
}

func (s *Standard) Stream(ctx context.Context, request atom.Request) (harness.Stream, error) {
	payload := map[string]any{
		"model":          request.Model,
		"messages":       s.messages(request.Messages),
		"stream":         true,
		"stream_options": map[string]any{"include_usage": true},
	}
	if len(request.Tools) > 0 {
		payload["tools"] = s.tools(request.Tools)
	}
	for key, value := range request.Params {
		payload[key] = value
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	url := strings.TrimSuffix(s.spec.APIURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if key := s.secret(ctx); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	response, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 8192))
		_ = response.Body.Close()
		return nil, fmt.Errorf("provider: the API gives the status %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
	}
	stream := &httpStream{parts: make(chan atom.ResponsePart, 64)}
	go parseSSE(response.Body, stream)
	return stream, nil
}

func (s *Standard) Refresh(ctx context.Context) ([]atom.ModelInfo, error) {
	if s.spec.ModelListURL == "" {
		return nil, errors.New("provider: the model list URL is not in the provider data")
	}
	prices, err := s.prices(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.spec.ModelListURL, nil)
	if err != nil {
		return nil, err
	}
	if key := s.secret(ctx); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	response, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("provider: the model list gives the status %d", response.StatusCode)
	}
	var list struct {
		Data []struct {
			ID      string `json:"id"`
			Pricing *struct {
				Prompt          string `json:"prompt"`
				Completion      string `json:"completion"`
				InputCacheRead  string `json:"input_cache_read"`
				InputCacheWrite string `json:"input_cache_write"`
			} `json:"pricing"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&list); err != nil {
		return nil, err
	}
	previous := map[string]atom.ModelInfo{}
	for _, model := range s.Models() {
		previous[model.ID] = model
	}
	var models []atom.ModelInfo
	for _, item := range list.Data {
		if item.ID == "" {
			continue
		}
		model := atom.ModelInfo{
			ID:     item.ID,
			Input:  []atom.MediaType{atom.Text},
			Output: []atom.MediaType{atom.Text},
		}
		if old, ok := previous[item.ID]; ok {
			model = old
		}
		if item.Pricing != nil {
			model.Prices = &atom.Prices{
				Currency:   "USD",
				Input:      perMillion(item.Pricing.Prompt),
				Output:     perMillion(item.Pricing.Completion),
				CacheRead:  perMillion(item.Pricing.InputCacheRead),
				CacheWrite: perMillion(item.Pricing.InputCacheWrite),
			}
		}
		if model.Prices == nil {
			if price, ok := prices[item.ID]; ok {
				model.Prices = &price
			}
		}
		models = append(models, model)
	}
	s.SetModels(models)
	return models, nil
}

func (s *Standard) prices(ctx context.Context) (map[string]atom.Prices, error) {
	result := map[string]atom.Prices{}
	s.mu.RLock()
	for id, price := range s.inlinePrices {
		result[id] = price
	}
	s.mu.RUnlock()
	source := s.spec.PriceTableURL
	if source == "" {
		return result, nil
	}
	var data []byte
	var err error
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if err != nil {
			return nil, err
		}
		response, err := s.client.Do(req)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("provider: the price table gives the status %d", response.StatusCode)
		}
		data, err = io.ReadAll(io.LimitReader(response.Body, 1<<20))
		if err != nil {
			return nil, err
		}
	} else {
		data, err = os.ReadFile(source)
		if err != nil {
			return nil, err
		}
	}
	var raw map[string]struct {
		Currency   string  `json:"currency"`
		Input      float64 `json:"input"`
		Output     float64 `json:"output"`
		CacheRead  float64 `json:"cache_read"`
		CacheWrite float64 `json:"cache_write"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	for id, entry := range raw {
		currency := entry.Currency
		if currency == "" {
			currency = "USD"
		}
		result[id] = atom.Prices{
			Currency:   currency,
			Input:      entry.Input,
			Output:     entry.Output,
			CacheRead:  entry.CacheRead,
			CacheWrite: entry.CacheWrite,
		}
	}
	return result, nil
}

func (s *Standard) messages(messages []atom.Message) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		item := map[string]any{"role": string(message.Role)}
		switch message.Role {
		case atom.RoleTool:
			item["tool_call_id"] = message.ToolCallID
			item["content"] = textOf(message.Content)
		case atom.RoleAssistant:
			item["content"] = textOf(message.Content)
			if len(message.ToolCalls) > 0 {
				calls := make([]map[string]any, 0, len(message.ToolCalls))
				for _, call := range message.ToolCalls {
					calls = append(calls, map[string]any{
						"id":   call.ID,
						"type": "function",
						"function": map[string]any{
							"name":      call.Name,
							"arguments": string(call.Input),
						},
					})
				}
				item["tool_calls"] = calls
			}
		default:
			item["content"] = contentOf(message.Content)
		}
		out = append(out, item)
	}
	return out
}

func contentOf(content []atom.Content) any {
	hasMedia := false
	for _, item := range content {
		if item.Type != atom.Text {
			hasMedia = true
			break
		}
	}
	if !hasMedia {
		return textOf(content)
	}
	parts := make([]map[string]any, 0, len(content))
	for _, item := range content {
		switch item.Type {
		case atom.Text:
			parts = append(parts, map[string]any{"type": "text", "text": item.Text})
		case atom.Image:
			mime := item.MIME
			if mime == "" {
				mime = "image/png"
			}
			data := base64.StdEncoding.EncodeToString(item.Data)
			parts = append(parts, map[string]any{
				"type": "image_url",
				"image_url": map[string]any{
					"url": "data:" + mime + ";base64," + data,
				},
			})
		default:
			parts = append(parts, map[string]any{"type": "text", "text": item.Text})
		}
	}
	return parts
}

func textOf(content []atom.Content) string {
	var builder strings.Builder
	for _, item := range content {
		if item.Type != atom.Text {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(item.Text)
	}
	return builder.String()
}

func (s *Standard) tools(tools []atom.ToolSpec) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		var parameters any = map[string]any{"type": "object"}
		if len(tool.InputSchema.JSON) > 0 {
			_ = json.Unmarshal(tool.InputSchema.JSON, &parameters)
		}
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  parameters,
			},
		})
	}
	return out
}

func perMillion(value string) float64 {
	if value == "" {
		return 0
	}
	var number float64
	if _, err := fmt.Sscanf(value, "%f", &number); err != nil {
		return 0
	}
	return number * 1_000_000
}

type httpStream struct {
	parts chan atom.ResponsePart
	err   error
}

func (s *httpStream) Recv(ctx context.Context) (atom.ResponsePart, error) {
	select {
	case part, ok := <-s.parts:
		if !ok {
			if s.err != nil {
				return atom.ResponsePart{}, s.err
			}
			return atom.ResponsePart{}, io.EOF
		}
		return part, nil
	case <-ctx.Done():
		return atom.ResponsePart{}, ctx.Err()
	}
}

type sseChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
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
	Usage *struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		PromptTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
		CompletionTokensDetails struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
	} `json:"usage"`
}

type toolAccumulator struct {
	id        string
	name      string
	arguments strings.Builder
	emitted   bool
}

func parseSSE(body io.ReadCloser, stream *httpStream) {
	defer body.Close()
	defer close(stream.parts)
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	accumulators := map[int]*toolAccumulator{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk sseChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				stream.parts <- atom.ResponsePart{Text: choice.Delta.Content}
			}
			for _, call := range choice.Delta.ToolCalls {
				accumulator, ok := accumulators[call.Index]
				if !ok {
					accumulator = &toolAccumulator{}
					accumulators[call.Index] = accumulator
				}
				if call.ID != "" {
					accumulator.id = call.ID
				}
				if call.Function.Name != "" {
					accumulator.name = call.Function.Name
				}
				accumulator.arguments.WriteString(call.Function.Arguments)
				if !accumulator.emitted && accumulator.name != "" && json.Valid([]byte(accumulator.arguments.String())) {
					accumulator.emitted = true
					stream.parts <- atom.ResponsePart{ToolCall: &atom.ToolCall{
						ID:    accumulator.id,
						Name:  accumulator.name,
						Input: []byte(accumulator.arguments.String()),
					}}
				}
			}
		}
		if chunk.Usage != nil {
			cached := chunk.Usage.PromptTokensDetails.CachedTokens
			usage := atom.Usage{
				Input:     chunk.Usage.PromptTokens - cached,
				CacheRead: cached,
				Output:    chunk.Usage.CompletionTokens,
				Reasoning: chunk.Usage.CompletionTokensDetails.ReasoningTokens,
			}
			stream.parts <- atom.ResponsePart{Usage: &usage}
		}
	}
	if err := scanner.Err(); err != nil {
		stream.err = err
	}
	for index := 0; index < len(accumulators); index++ {
		accumulator, ok := accumulators[index]
		if !ok || accumulator.emitted || accumulator.name == "" {
			continue
		}
		if !json.Valid([]byte(accumulator.arguments.String())) {
			stream.err = fmt.Errorf("provider: the tool call %q has input which is not correct JSON", accumulator.name)
			return
		}
		stream.parts <- atom.ResponsePart{ToolCall: &atom.ToolCall{
			ID:    accumulator.id,
			Name:  accumulator.name,
			Input: []byte(accumulator.arguments.String()),
		}}
	}
}

var _ = time.Second
