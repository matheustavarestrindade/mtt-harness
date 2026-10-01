package providerauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/openaiauth"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestRefreshIsSerializedAndDisconnectRemovesCredentials(test *testing.T) {
	operationContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var refreshes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/oauth/token" || request.ParseForm() != nil || request.Form.Get("refresh_token") != "old-refresh" {
			http.Error(responseWriter, "invalid refresh", 400)
			return
		}
		refreshes.Add(1)
		_ = json.NewEncoder(responseWriter).Encode(map[string]any{"access_token": "fresh-access", "refresh_token": "rotated-refresh", "expires_in": 3600})
	}))
	defer server.Close()
	database := memory.New()
	testutil.RequireNoError(test, database.Secrets().SaveOAuthCredential(operationContext, provider.CodexProvider, atom.OAuthCredential{AccessToken: "expired", RefreshToken: "old-refresh", AccountID: "account", ExpiresAt: time.Now().Add(-time.Minute)}))
	service := New(operationContext, database.Secrets(), &openaiauth.Client{Issuer: server.URL, HTTP: server.Client()})
	defer service.Close()
	var workers sync.WaitGroup
	failures := make(chan error, 20)
	for index := 0; index < 20; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			headers, operationError := service.Headers(operationContext)
			if operationError != nil {
				failures <- operationError
				return
			}
			if headers.Get("Authorization") != "Bearer fresh-access" || headers.Get("ChatGPT-Account-Id") != "account" {
				test.Error("authentication headers were not restored")
			}
		}()
	}
	workers.Wait()
	close(failures)
	for operationError := range failures {
		testutil.RequireNoError(test, operationError)
	}
	if refreshes.Load() != 1 {
		test.Fatalf("concurrent refreshes = %d", refreshes.Load())
	}
	testutil.RequireNoError(test, service.Disconnect(operationContext))
	if _, operationError := service.Headers(operationContext); operationError == nil {
		test.Fatal("disconnected provider still has headers")
	}
}

func TestCancelledAndSupersededLoginCannotSaveLateCredentials(test *testing.T) {
	operationContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/accounts/deviceauth/usercode" {
			_ = json.NewEncoder(responseWriter).Encode(map[string]any{"device_auth_id": "device", "user_code": "CODE", "interval": "60"})
			return
		}
		responseWriter.WriteHeader(403)
	}))
	defer server.Close()
	database := memory.New()
	service := New(operationContext, database.Secrets(), &openaiauth.Client{Issuer: server.URL, HTTP: server.Client()})
	defer service.Close()
	login, operationError := service.Start(operationContext)
	testutil.RequireNoError(test, operationError)
	old := service.flow
	testutil.RequireNoError(test, service.Cancel(operationContext, login.ID))
	credential := atom.OAuthCredential{AccessToken: "late", RefreshToken: "late-refresh", AccountID: "account", ExpiresAt: time.Now().Add(time.Hour)}
	service.finish(old, credential, nil)
	stored, operationError := database.Secrets().OAuthCredential(operationContext, provider.CodexProvider)
	testutil.RequireNoError(test, operationError)
	if stored.AccessToken != "" {
		test.Fatal("cancelled login saved credentials")
	}
	_, operationError = service.Start(operationContext)
	testutil.RequireNoError(test, operationError)
	service.finish(old, credential, nil)
	stored, operationError = database.Secrets().OAuthCredential(operationContext, provider.CodexProvider)
	testutil.RequireNoError(test, operationError)
	if stored.AccessToken != "" {
		test.Fatal("superseded login saved credentials")
	}
	status, operationError := service.Status(operationContext, service.flow.status.ID)
	testutil.RequireNoError(test, operationError)
	if status.Status != "pending" {
		test.Fatalf("new login = %#v", status)
	}
}

func TestDeviceLoginPersistsAcrossServiceRestart(test *testing.T) {
	operationContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	access := "header." + base64.RawURLEncoding.EncodeToString([]byte(`{"chatgpt_account_id":"account"}`)) + ".signature"
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/accounts/deviceauth/usercode":
			_ = json.NewEncoder(responseWriter).Encode(map[string]any{"device_auth_id": "private-device", "user_code": "PUBLIC-CODE", "interval": "1"})
		case "/api/accounts/deviceauth/token":
			_ = json.NewEncoder(responseWriter).Encode(map[string]string{"authorization_code": "code", "code_verifier": "verifier"})
		case "/oauth/token":
			_ = json.NewEncoder(responseWriter).Encode(map[string]any{"access_token": access, "refresh_token": "refresh", "expires_in": 3600})
		default:
			http.NotFound(responseWriter, request)
		}
	}))
	defer server.Close()
	database := memory.New()
	client := &openaiauth.Client{Issuer: server.URL, HTTP: server.Client()}
	service := New(operationContext, database.Secrets(), client)
	defer service.Close()
	login, operationError := service.Start(operationContext)
	testutil.RequireNoError(test, operationError)
	for {
		status, operationError := service.Status(operationContext, login.ID)
		testutil.RequireNoError(test, operationError)
		if status.Status != "pending" {
			if status.Status != "connected" || status.UserCode != "" {
				test.Fatalf("completed login = %#v", status)
			}
			break
		}
		select {
		case <-operationContext.Done():
			test.Fatal("device login did not complete")
		case <-time.After(10 * time.Millisecond):
		}
	}
	service.Close()
	restarted := New(operationContext, database.Secrets(), client)
	defer restarted.Close()
	headers, operationError := restarted.Headers(operationContext)
	testutil.RequireNoError(test, operationError)
	if headers.Get("Authorization") != "Bearer "+access || headers.Get("ChatGPT-Account-Id") != "account" {
		test.Fatal("completed login did not survive service restart")
	}
}
