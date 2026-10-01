package openaiauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestDeviceCodeExchangeAndRefresh(test *testing.T) {
	var pollCount atomic.Int32
	accessToken := "header." + base64.RawURLEncoding.EncodeToString([]byte(`{"https://api.openai.com/auth":{"chatgpt_account_id":"account","chatgpt_compute_residency":"us"}}`)) + ".signature"
	var issuer string
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/accounts/deviceauth/usercode":
			var body map[string]string
			if json.NewDecoder(request.Body).Decode(&body) != nil || body["client_id"] != OpenAICodexOAuthClientID {
				http.Error(responseWriter, "bad client", 400)
				return
			}
			writeAuthenticationResponse(responseWriter, map[string]any{"device_auth_id": "device-secret", "user_code": "TEST-CODE", "interval": "2"})
		case "/api/accounts/deviceauth/token":
			var body map[string]string
			if json.NewDecoder(request.Body).Decode(&body) != nil || body["device_auth_id"] != "device-secret" || body["user_code"] != "TEST-CODE" {
				http.Error(responseWriter, "bad device code", 400)
				return
			}
			if pollCount.Add(1) == 1 {
				responseWriter.WriteHeader(403)
				return
			}
			writeAuthenticationResponse(responseWriter, map[string]string{"authorization_code": "authorized", "code_verifier": "verifier"})
		case "/oauth/token":
			if request.ParseForm() != nil || request.Form.Get("client_id") != OpenAICodexOAuthClientID {
				http.Error(responseWriter, "bad form", 400)
				return
			}
			if request.Form.Get("grant_type") == "authorization_code" {
				if request.Form.Get("code") != "authorized" || request.Form.Get("code_verifier") != "verifier" || request.Form.Get("redirect_uri") != issuer+"/deviceauth/callback" {
					http.Error(responseWriter, "bad code exchange", 400)
					return
				}
				writeAuthenticationResponse(responseWriter, map[string]any{"access_token": accessToken, "refresh_token": "refresh-one", "expires_in": 3600})
				return
			}
			if request.Form.Get("grant_type") != "refresh_token" || request.Form.Get("refresh_token") != "refresh-one" {
				http.Error(responseWriter, "bad refresh", 400)
				return
			}
			writeAuthenticationResponse(responseWriter, map[string]any{"access_token": accessToken, "refresh_token": "refresh-two", "expires_in": 3600})
		default:
			http.NotFound(responseWriter, request)
		}
	}))
	defer server.Close()
	issuer = server.URL
	client := &Client{IssuerURL: issuer, HTTPClient: server.Client()}
	challenge, operationError := client.RequestDeviceAuthorization(context.Background())
	testutil.RequireNoError(test, operationError)
	if challenge.PollInterval != 2*time.Second || challenge.VerificationURL != issuer+"/codex/device" {
		test.Fatalf("challenge = %#v", challenge)
	}
	_, operationError = client.PollDeviceAuthorization(context.Background(), challenge)
	if !errors.Is(operationError, ErrPending) {
		test.Fatalf("pending = %v", operationError)
	}
	credential, operationError := client.PollDeviceAuthorization(context.Background(), challenge)
	testutil.RequireNoError(test, operationError)
	if credential.AccountID != "account" || credential.Residency != "us" || credential.RefreshToken != "refresh-one" {
		test.Fatal("credential metadata was not retained")
	}
	refreshed, operationError := client.RefreshAccessToken(context.Background(), credential)
	testutil.RequireNoError(test, operationError)
	if refreshed.RefreshToken != "refresh-two" || refreshed.AccountID != "account" {
		test.Fatal("refresh token rotation failed")
	}
	encoded, operationError := json.Marshal(refreshed)
	testutil.RequireNoError(test, operationError)
	if strings.Contains(string(encoded), "refresh-two") || strings.Contains(string(encoded), accessToken) {
		test.Fatal("token leaked into public JSON")
	}
}

func writeAuthenticationResponse(responseWriter http.ResponseWriter, value any) {
	_ = json.NewEncoder(responseWriter).Encode(value)
}

func TestAuthenticationErrorsDoNotIncludeResponseSecrets(test *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		http.Error(responseWriter, "secret-token-in-error-body", http.StatusUnauthorized)
	}))
	defer server.Close()
	client := &Client{IssuerURL: server.URL, HTTPClient: server.Client()}
	_, operationError := client.RefreshAccessToken(context.Background(), atom.OAuthCredential{RefreshToken: "refresh"})
	if operationError == nil || strings.Contains(operationError.Error(), "secret-token") {
		test.Fatalf("unsafe error: %v", operationError)
	}
}
