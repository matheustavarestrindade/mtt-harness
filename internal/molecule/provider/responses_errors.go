package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

const maximumProviderErrorBytes = 4096

type responsesErrorDetail struct {
	Type    string          `json:"type"`
	Code    json.RawMessage `json:"code"`
	Message string          `json:"message"`
	Param   string          `json:"param"`
}

func (detail responsesErrorDetail) code() string {
	var value string
	if json.Unmarshal(detail.Code, &value) == nil {
		return value
	}
	return string(detail.Code)
}

// responseTransportError keeps observed transport facts. It never classifies a
// 503 as a size/authentication error or includes credentials from echoed errors.
type responseTransportError struct {
	provider     string
	transport    string
	status       int
	requestID    string
	retryAfter   string
	requestBytes int
	detail       responsesErrorDetail
}

func (operationError *responseTransportError) Error() string {
	text := fmt.Sprintf("%s Responses %s returned HTTP %d; request_bytes=%d", operationError.provider, operationError.transport, operationError.status, operationError.requestBytes)
	if operationError.requestID != "" {
		text += "; request_id=" + operationError.requestID
	}
	if operationError.retryAfter != "" {
		text += "; retry_after=" + operationError.retryAfter
	}
	if code := operationError.detail.code(); code != "" && code != "null" {
		text += "; code=" + code
	}
	if operationError.detail.Message != "" {
		text += ": " + operationError.detail.Message
	}
	return text
}

func responsesHTTPError(providerName, transport string, response *http.Response, requestBytes int, headers http.Header) error {
	operationError := &responseTransportError{provider: providerName, transport: transport, status: response.StatusCode, requestBytes: requestBytes,
		requestID: redactProviderDetail(response.Header.Get("x-request-id"), headers), retryAfter: redactProviderDetail(response.Header.Get("Retry-After"), headers)}
	if operationError.requestID == "" {
		operationError.requestID = redactProviderDetail(response.Header.Get("request-id"), headers)
	}
	if response.Body == nil {
		return operationError
	}
	defer response.Body.Close()
	readDeadline := time.AfterFunc(3*time.Second, func() { response.Body.Close() })
	defer readDeadline.Stop()
	body, readError := io.ReadAll(io.LimitReader(response.Body, maximumProviderErrorBytes))
	if readError != nil {
		return fmt.Errorf("%w; read provider error body: %v", operationError, readError)
	}
	var envelope struct {
		Error responsesErrorDetail `json:"error"`
	}
	if json.Unmarshal(body, &envelope) == nil {
		operationError.detail = envelope.Error
		operationError.detail.Message = redactProviderDetail(envelope.Error.Message, headers)
		operationError.detail.Code = nil
		if code := redactProviderDetail(envelope.Error.code(), headers); code != "" {
			operationError.detail.Code, _ = json.Marshal(code)
		}
	}
	return operationError
}

func boundedProviderDetail(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 2048 {
		text = text[:2048]
		for !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
		text += "…"
	}
	return text
}

func redactProviderDetail(text string, headers http.Header) string {
	for name, values := range headers {
		lowerName := strings.ToLower(name)
		if lowerName != "authorization" && lowerName != "cookie" && !strings.Contains(lowerName, "key") && !strings.Contains(lowerName, "token") {
			continue
		}
		for _, value := range values {
			if value != "" {
				text = strings.ReplaceAll(text, value, "[redacted]")
			}
			if token, found := strings.CutPrefix(value, "Bearer "); found && token != "" {
				text = strings.ReplaceAll(text, token, "[redacted]")
			}
		}
	}
	return boundedProviderDetail(text)
}
