// Package openaiauth implements OpenAI's headless Codex device-code protocol.
// Source: openai/codex, codex-rs/login/src/device_code_auth.rs.
package openaiauth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

const DefaultIssuer = "https://auth.openai.com"
const ClientID = "app_EMoamEEZ73f0CkXaXp7hrann"

var ErrPending = errors.New("OpenAI authorization is pending")

type DeviceChallenge struct {
	DeviceID        string
	UserCode        string
	VerificationURL string
	Interval        time.Duration
	ExpiresAt       time.Time
}

// Client has injectable endpoints for protocol tests. Runtime wiring uses the
// fixed OpenAI issuer, not a browser-supplied redirect or authentication URL.
type Client struct {
	Issuer string
	HTTP   *http.Client
}

func New() *Client {
	return &Client{Issuer: DefaultIssuer, HTTP: &http.Client{Timeout: 20 * time.Second}}
}

func (client *Client) Start(operationContext context.Context) (DeviceChallenge, error) {
	var response struct {
		DeviceID      string          `json:"device_auth_id"`
		UserCode      string          `json:"user_code"`
		UserCodeAlias string          `json:"usercode"`
		Interval      json.RawMessage `json:"interval"`
	}
	_, operationError := client.post(operationContext, "/api/accounts/deviceauth/usercode", "application/json", []byte(`{"client_id":"`+ClientID+`"}`), &response)
	if operationError != nil {
		return DeviceChallenge{}, operationError
	}
	if response.UserCode == "" {
		response.UserCode = response.UserCodeAlias
	}
	if response.DeviceID == "" || response.UserCode == "" {
		return DeviceChallenge{}, errors.New("OpenAI returned an incomplete device code")
	}
	seconds, operationError := strconv.Atoi(strings.Trim(string(response.Interval), `"`))
	if operationError != nil || seconds < 1 {
		seconds = 5
	}
	if seconds > 60 {
		seconds = 60
	}
	return DeviceChallenge{DeviceID: response.DeviceID, UserCode: response.UserCode,
		VerificationURL: strings.TrimRight(client.Issuer, "/") + "/codex/device",
		Interval:        time.Duration(seconds) * time.Second, ExpiresAt: time.Now().Add(15 * time.Minute)}, nil
}

func (client *Client) Poll(operationContext context.Context, challenge DeviceChallenge) (atom.OAuthCredential, error) {
	body, operationError := json.Marshal(map[string]string{"device_auth_id": challenge.DeviceID, "user_code": challenge.UserCode})
	if operationError != nil {
		return atom.OAuthCredential{}, operationError
	}
	var authorization struct {
		Code     string `json:"authorization_code"`
		Verifier string `json:"code_verifier"`
	}
	status, operationError := client.post(operationContext, "/api/accounts/deviceauth/token", "application/json", body, &authorization)
	if status == http.StatusForbidden || status == http.StatusNotFound {
		return atom.OAuthCredential{}, ErrPending
	}
	if operationError != nil {
		return atom.OAuthCredential{}, operationError
	}
	if authorization.Code == "" || authorization.Verifier == "" {
		return atom.OAuthCredential{}, errors.New("OpenAI returned an incomplete authorization")
	}
	values := url.Values{"grant_type": {"authorization_code"}, "client_id": {ClientID}, "code": {authorization.Code}, "code_verifier": {authorization.Verifier}, "redirect_uri": {strings.TrimRight(client.Issuer, "/") + "/deviceauth/callback"}}
	return client.exchange(operationContext, values, atom.OAuthCredential{})
}

func (client *Client) Refresh(operationContext context.Context, credential atom.OAuthCredential) (atom.OAuthCredential, error) {
	if credential.RefreshToken == "" {
		return atom.OAuthCredential{}, errors.New("sign in with ChatGPT again; no refresh token is available")
	}
	return client.exchange(operationContext, url.Values{"grant_type": {"refresh_token"}, "client_id": {ClientID}, "refresh_token": {credential.RefreshToken}}, credential)
}

func (client *Client) exchange(operationContext context.Context, values url.Values, previous atom.OAuthCredential) (atom.OAuthCredential, error) {
	var response struct {
		Access    string `json:"access_token"`
		Refresh   string `json:"refresh_token"`
		IDToken   string `json:"id_token"`
		ExpiresIn int64  `json:"expires_in"`
	}
	_, operationError := client.post(operationContext, "/oauth/token", "application/x-www-form-urlencoded", []byte(values.Encode()), &response)
	if operationError != nil {
		return atom.OAuthCredential{}, operationError
	}
	if response.Access == "" {
		return atom.OAuthCredential{}, errors.New("OpenAI returned no access token")
	}
	credential := previous
	credential.AccessToken = response.Access
	if response.Refresh != "" {
		credential.RefreshToken = response.Refresh
	}
	if credential.RefreshToken == "" {
		return atom.OAuthCredential{}, errors.New("OpenAI returned no refresh token")
	}
	if response.ExpiresIn <= 0 {
		response.ExpiresIn = 3600
	}
	if response.ExpiresIn > 365*24*3600 {
		return atom.OAuthCredential{}, errors.New("OpenAI returned an invalid token expiry")
	}
	credential.ExpiresAt = time.Now().Add(time.Duration(response.ExpiresIn) * time.Second)
	for _, token := range []string{response.IDToken, response.Access} {
		account, residency := claims(token)
		if account != "" {
			credential.AccountID = account
		}
		if residency != "" {
			credential.Residency = residency
		}
	}
	if credential.AccountID == "" {
		return atom.OAuthCredential{}, errors.New("OpenAI returned no ChatGPT account ID")
	}
	return credential, nil
}

func (client *Client) post(operationContext context.Context, path string, contentType string, body []byte, result any) (int, error) {
	request, operationError := http.NewRequestWithContext(operationContext, http.MethodPost, strings.TrimRight(client.Issuer, "/")+path, bytes.NewReader(body))
	if operationError != nil {
		return 0, operationError
	}
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("User-Agent", "mtt-harness/1.0")
	response, operationError := client.HTTP.Do(request)
	if operationError != nil {
		return 0, fmt.Errorf("OpenAI authentication request failed: %w", operationError)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response.StatusCode, fmt.Errorf("OpenAI authentication returned HTTP %d", response.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1024*1024))
	if operationError := decoder.Decode(result); operationError != nil {
		return response.StatusCode, fmt.Errorf("invalid OpenAI authentication response: %w", operationError)
	}
	return response.StatusCode, nil
}

// Claims are read only from tokens returned by the HTTPS OAuth exchange. They
// provide routing metadata, not an independent authentication decision.
func claims(token string) (string, string) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", ""
	}
	data, operationError := base64.RawURLEncoding.DecodeString(parts[1])
	if operationError != nil {
		return "", ""
	}
	type routing struct {
		AccountID string `json:"chatgpt_account_id"`
		Residency string `json:"chatgpt_compute_residency"`
	}
	var payload struct {
		routing
		Auth          routing `json:"https://api.openai.com/auth"`
		Organizations []struct {
			ID string `json:"id"`
		} `json:"organizations"`
	}
	if json.Unmarshal(data, &payload) != nil {
		return "", ""
	}
	account, residency := payload.AccountID, payload.Residency
	if account == "" {
		account = payload.Auth.AccountID
	}
	if residency == "" {
		residency = payload.Auth.Residency
	}
	if account == "" && len(payload.Organizations) > 0 {
		account = payload.Organizations[0].ID
	}
	if residency == "no_constraint" {
		residency = ""
	}
	return account, residency
}
