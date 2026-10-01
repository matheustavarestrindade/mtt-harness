// Package providerauth owns pending provider logins and serialized OAuth token
// rotation. It never exposes access or refresh tokens to API clients.
package providerauth

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/openaiauth"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

var ErrLoginNotFound = errors.New("provider login is not available; start sign-in again")

type loginFlow struct {
	status atom.DeviceLogin
	cancel context.CancelFunc
}

// Service's gate serializes credential replacement with refresh and disconnect.
// The polling worker performs network I/O outside the gate, then checks its
// generation before saving credentials. Close cancels and joins that worker.
type Service struct {
	context context.Context
	cancel  context.CancelFunc
	secrets store.SecretStore
	client  *openaiauth.Client
	gate    chan struct{}
	flow    *loginFlow
	closed  bool
	workers sync.WaitGroup
}

func New(operationContext context.Context, secrets store.SecretStore, client *openaiauth.Client) *Service {
	if client == nil {
		client = openaiauth.New()
	}
	lifetime, cancel := context.WithCancel(operationContext)
	return &Service{context: lifetime, cancel: cancel, secrets: secrets, client: client, gate: make(chan struct{}, 1)}
}

func (service *Service) acquire(operationContext context.Context) error {
	select {
	case service.gate <- struct{}{}:
		if operationError := operationContext.Err(); operationError != nil {
			service.release()
			return operationError
		}
		return nil
	case <-operationContext.Done():
		return operationContext.Err()
	}
}

func (service *Service) release() { <-service.gate }

func (service *Service) Start(operationContext context.Context) (atom.DeviceLogin, error) {
	if operationError := service.acquire(operationContext); operationError != nil {
		return atom.DeviceLogin{}, operationError
	}
	defer service.release()
	if service.closed || service.context.Err() != nil {
		return atom.DeviceLogin{}, errors.New("provider authentication is shutting down")
	}
	challenge, operationError := service.client.Start(operationContext)
	if operationError != nil {
		return atom.DeviceLogin{}, operationError
	}
	if service.flow != nil {
		service.flow.cancel()
	}
	loginContext, cancel := context.WithDeadline(service.context, challenge.ExpiresAt)
	flow := &loginFlow{cancel: cancel, status: atom.DeviceLogin{
		ID: rand.Text(), Provider: provider.CodexProvider, VerificationURL: challenge.VerificationURL,
		UserCode: challenge.UserCode, ExpiresAt: challenge.ExpiresAt, Status: "pending",
	}}
	service.flow = flow
	service.workers.Add(1)
	go service.poll(loginContext, flow, challenge)
	return flow.status, nil
}

func (service *Service) Status(operationContext context.Context, identifier string) (atom.DeviceLogin, error) {
	if operationError := service.acquire(operationContext); operationError != nil {
		return atom.DeviceLogin{}, operationError
	}
	defer service.release()
	if service.flow == nil || service.flow.status.ID != identifier {
		return atom.DeviceLogin{}, ErrLoginNotFound
	}
	return service.flow.status, nil
}

func (service *Service) Cancel(operationContext context.Context, identifier string) error {
	if operationError := service.acquire(operationContext); operationError != nil {
		return operationError
	}
	defer service.release()
	if service.flow == nil || service.flow.status.ID != identifier {
		return ErrLoginNotFound
	}
	if service.flow.status.Status != "pending" {
		return nil
	}
	service.flow.cancel()
	service.flow.status.Status = "cancelled"
	return nil
}

func (service *Service) Disconnect(operationContext context.Context) error {
	if operationError := service.acquire(operationContext); operationError != nil {
		return operationError
	}
	defer service.release()
	if service.flow != nil {
		service.flow.cancel()
		service.flow.status.Status = "cancelled"
	}
	return service.secrets.DeleteOAuthCredential(operationContext, provider.CodexProvider)
}

func (service *Service) Headers(operationContext context.Context) (http.Header, error) {
	if operationError := service.acquire(operationContext); operationError != nil {
		return nil, operationError
	}
	defer service.release()
	credential, operationError := service.secrets.OAuthCredential(operationContext, provider.CodexProvider)
	if operationError != nil {
		return nil, operationError
	}
	if credential.AccessToken == "" {
		return nil, errors.New("connect the OpenAI coding plan in Providers")
	}
	if time.Until(credential.ExpiresAt) < time.Minute {
		credential, operationError = service.client.Refresh(operationContext, credential)
		if operationError != nil {
			return nil, operationError
		}
		if operationError := service.secrets.SaveOAuthCredential(operationContext, provider.CodexProvider, credential); operationError != nil {
			return nil, operationError
		}
	}
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+credential.AccessToken)
	headers.Set("ChatGPT-Account-Id", credential.AccountID)
	headers.Set("originator", "mtt-harness")
	headers.Set("User-Agent", "mtt-harness/1.0")
	if credential.Residency != "" {
		headers.Set("x-openai-internal-codex-residency", credential.Residency)
	}
	return headers, nil
}

func (service *Service) Close() {
	service.cancel()
	// This short owner join uses no request context; shutdown is irreversible.
	service.gate <- struct{}{}
	service.closed = true
	if service.flow != nil {
		service.flow.cancel()
	}
	service.release()
	service.workers.Wait()
}
