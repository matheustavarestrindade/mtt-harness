// Package openaiauth implements OpenAI's headless Codex device-code protocol.
// Source: openai/codex, codex-rs/login/src/device_code_auth.rs.
package openaiauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const DefaultIssuerURL = "https://auth.openai.com"

// OpenAICodexOAuthClientID is the public application's identifier, not a secret.
// Source: https://github.com/anomalyco/opencode/blob/dev/packages/opencode/src/plugin/openai/codex.ts
// OAuth client identifiers are public (RFC 6749, section 2.2).
const OpenAICodexOAuthClientID = "app_EMoamEEZ73f0CkXaXp7hrann"

var ErrPending = errors.New("OpenAI authorization is pending")

// Client has injectable endpoints for protocol tests. Runtime wiring uses the
// fixed OpenAI issuer, not a browser-supplied redirect or authentication URL.
type Client struct {
	IssuerURL  string
	HTTPClient *http.Client
}

func New() *Client {
	return &Client{IssuerURL: DefaultIssuerURL, HTTPClient: &http.Client{Timeout: 20 * time.Second}}
}

func (authenticationClient *Client) sendAuthenticationRequest(operationContext context.Context, endpointPath, contentType string, requestBody []byte, decodedResponse any) (int, error) {
	request, operationError := http.NewRequestWithContext(operationContext, http.MethodPost, strings.TrimRight(authenticationClient.IssuerURL, "/")+endpointPath, bytes.NewReader(requestBody))
	if operationError != nil {
		return 0, operationError
	}
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("User-Agent", "mtt-harness/1.0")
	response, operationError := authenticationClient.HTTPClient.Do(request)
	if operationError != nil {
		return 0, fmt.Errorf("OpenAI authentication request failed: %w", operationError)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response.StatusCode, fmt.Errorf("OpenAI authentication returned HTTP %d", response.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1024*1024))
	if operationError := decoder.Decode(decodedResponse); operationError != nil {
		return response.StatusCode, fmt.Errorf("invalid OpenAI authentication response: %w", operationError)
	}
	return response.StatusCode, nil
}
