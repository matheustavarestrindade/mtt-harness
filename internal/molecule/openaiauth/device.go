package openaiauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type DeviceChallenge struct {
	DeviceID        string
	UserCode        string
	VerificationURL string
	PollInterval    time.Duration
	ExpiresAt       time.Time
}

// RequestDeviceAuthorization obtains a challenge without waiting for user login.
func (authenticationClient *Client) RequestDeviceAuthorization(operationContext context.Context) (DeviceChallenge, error) {
	var response struct {
		DeviceID      string          `json:"device_auth_id"`
		UserCode      string          `json:"user_code"`
		UserCodeAlias string          `json:"usercode"`
		PollInterval  json.RawMessage `json:"interval"`
	}
	_, operationError := authenticationClient.sendAuthenticationRequest(operationContext, "/api/accounts/deviceauth/usercode", "application/json", []byte(`{"client_id":"`+OpenAICodexOAuthClientID+`"}`), &response)
	if operationError != nil {
		return DeviceChallenge{}, operationError
	}
	if response.UserCode == "" {
		response.UserCode = response.UserCodeAlias
	}
	if response.DeviceID == "" || response.UserCode == "" {
		return DeviceChallenge{}, errors.New("OpenAI returned an incomplete device code")
	}
	pollIntervalSeconds, operationError := strconv.Atoi(strings.Trim(string(response.PollInterval), `"`))
	if operationError != nil || pollIntervalSeconds < 1 {
		pollIntervalSeconds = 5
	}
	if pollIntervalSeconds > 60 {
		pollIntervalSeconds = 60
	}
	return DeviceChallenge{DeviceID: response.DeviceID, UserCode: response.UserCode,
		VerificationURL: strings.TrimRight(authenticationClient.IssuerURL, "/") + "/codex/device",
		PollInterval:    time.Duration(pollIntervalSeconds) * time.Second, ExpiresAt: time.Now().Add(15 * time.Minute)}, nil
}

// PollDeviceAuthorization checks once. ErrPending means that the caller must wait
// for the challenge's poll interval before checking again.
func (authenticationClient *Client) PollDeviceAuthorization(operationContext context.Context, challenge DeviceChallenge) (atom.OAuthCredential, error) {
	requestBody, operationError := json.Marshal(map[string]string{"device_auth_id": challenge.DeviceID, "user_code": challenge.UserCode})
	if operationError != nil {
		return atom.OAuthCredential{}, operationError
	}
	var authorization struct {
		AuthorizationCode string `json:"authorization_code"`
		CodeVerifier      string `json:"code_verifier"`
	}
	statusCode, operationError := authenticationClient.sendAuthenticationRequest(operationContext, "/api/accounts/deviceauth/token", "application/json", requestBody, &authorization)
	if statusCode == http.StatusForbidden || statusCode == http.StatusNotFound {
		return atom.OAuthCredential{}, ErrPending
	}
	if operationError != nil {
		return atom.OAuthCredential{}, operationError
	}
	if authorization.AuthorizationCode == "" || authorization.CodeVerifier == "" {
		return atom.OAuthCredential{}, errors.New("OpenAI returned an incomplete authorization")
	}
	tokenRequestParameters := url.Values{"grant_type": {"authorization_code"}, "client_id": {OpenAICodexOAuthClientID}, "code": {authorization.AuthorizationCode}, "code_verifier": {authorization.CodeVerifier}, "redirect_uri": {strings.TrimRight(authenticationClient.IssuerURL, "/") + "/deviceauth/callback"}}
	return authenticationClient.requestOAuthTokens(operationContext, tokenRequestParameters, atom.OAuthCredential{})
}
