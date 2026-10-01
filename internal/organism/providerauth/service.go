// Package providerauth owns pending provider logins and serialized OAuth token
// rotation. It never exposes access or refresh tokens to API clients.
package providerauth

import (
	"context"
	"errors"
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/openaiauth"
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
	lifetimeContext      context.Context
	cancelLifetime       context.CancelFunc
	secretStore          store.SecretStore
	authenticationClient *openaiauth.Client
	credentialGate       chan struct{}
	activeLogin          *loginFlow
	closed               bool
	pollingWorkers       sync.WaitGroup
}

func New(operationContext context.Context, secretStore store.SecretStore, authenticationClient *openaiauth.Client) *Service {
	if authenticationClient == nil {
		authenticationClient = openaiauth.New()
	}
	lifetimeContext, cancelLifetime := context.WithCancel(operationContext)
	return &Service{lifetimeContext: lifetimeContext, cancelLifetime: cancelLifetime, secretStore: secretStore, authenticationClient: authenticationClient, credentialGate: make(chan struct{}, 1)}
}

func (authenticationService *Service) acquireCredentialGate(operationContext context.Context) error {
	select {
	case authenticationService.credentialGate <- struct{}{}:
		if operationError := operationContext.Err(); operationError != nil {
			authenticationService.releaseCredentialGate()
			return operationError
		}
		return nil
	case <-operationContext.Done():
		return operationContext.Err()
	}
}

func (authenticationService *Service) releaseCredentialGate() { <-authenticationService.credentialGate }

func (authenticationService *Service) Close() {
	authenticationService.cancelLifetime()
	// This short owner join uses no request context; shutdown is irreversible.
	authenticationService.credentialGate <- struct{}{}
	authenticationService.closed = true
	if authenticationService.activeLogin != nil {
		authenticationService.activeLogin.cancel()
	}
	authenticationService.releaseCredentialGate()
	authenticationService.pollingWorkers.Wait()
}
