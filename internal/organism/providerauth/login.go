package providerauth

import (
	"context"
	"crypto/rand"
	"errors"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

// StartDeviceLogin replaces the active challenge and starts server-side polling.
func (authenticationService *Service) StartDeviceLogin(operationContext context.Context, providerID string) (atom.DeviceLogin, error) {
	if operationError := authenticationService.acquireCredentialGate(operationContext); operationError != nil {
		return atom.DeviceLogin{}, operationError
	}
	defer authenticationService.releaseCredentialGate()
	if authenticationService.closed || authenticationService.lifetimeContext.Err() != nil {
		return atom.DeviceLogin{}, errors.New("provider authentication is shutting down")
	}
	challenge, operationError := authenticationService.authenticationClient.RequestDeviceAuthorization(operationContext)
	if operationError != nil {
		return atom.DeviceLogin{}, operationError
	}
	if authenticationService.activeLogin != nil {
		authenticationService.activeLogin.cancel()
	}
	loginContext, cancelLogin := context.WithDeadline(authenticationService.lifetimeContext, challenge.ExpiresAt)
	pendingLogin := &loginFlow{cancel: cancelLogin, status: atom.DeviceLogin{
		ID: rand.Text(), Provider: providerID, VerificationURL: challenge.VerificationURL,
		UserCode: challenge.UserCode, ExpiresAt: challenge.ExpiresAt, Status: "pending",
	}}
	authenticationService.activeLogin = pendingLogin
	authenticationService.pollingWorkers.Add(1)
	go authenticationService.pollDeviceAuthorization(loginContext, pendingLogin, challenge)
	return pendingLogin.status, nil
}

func (authenticationService *Service) DeviceLoginStatus(operationContext context.Context, providerID, loginID string) (atom.DeviceLogin, error) {
	if operationError := authenticationService.acquireCredentialGate(operationContext); operationError != nil {
		return atom.DeviceLogin{}, operationError
	}
	defer authenticationService.releaseCredentialGate()
	if authenticationService.activeLogin == nil || authenticationService.activeLogin.status.ID != loginID || authenticationService.activeLogin.status.Provider != providerID {
		return atom.DeviceLogin{}, ErrLoginNotFound
	}
	return authenticationService.activeLogin.status, nil
}

// CancelDeviceLogin prevents the polling worker from storing late credentials.
func (authenticationService *Service) CancelDeviceLogin(operationContext context.Context, providerID, loginID string) error {
	if operationError := authenticationService.acquireCredentialGate(operationContext); operationError != nil {
		return operationError
	}
	defer authenticationService.releaseCredentialGate()
	if authenticationService.activeLogin == nil || authenticationService.activeLogin.status.ID != loginID || authenticationService.activeLogin.status.Provider != providerID {
		return ErrLoginNotFound
	}
	if authenticationService.activeLogin.status.Status != "pending" {
		return nil
	}
	authenticationService.activeLogin.cancel()
	authenticationService.activeLogin.status.Status = "cancelled"
	return nil
}
