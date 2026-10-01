package providerauth

import (
	"context"
	"errors"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/openaiauth"
)

func (authenticationService *Service) pollDeviceAuthorization(operationContext context.Context, pendingLogin *loginFlow, challenge openaiauth.DeviceChallenge) {
	defer authenticationService.pollingWorkers.Done()
	defer pendingLogin.cancel()
	for {
		pollTimer := time.NewTimer(challenge.PollInterval)
		select {
		case <-operationContext.Done():
			pollTimer.Stop()
			authenticationService.completeDeviceLogin(pendingLogin, atom.OAuthCredential{}, operationContext.Err())
			return
		case <-pollTimer.C:
		}
		credential, operationError := authenticationService.authenticationClient.PollDeviceAuthorization(operationContext, challenge)
		if errors.Is(operationError, openaiauth.ErrPending) {
			continue
		}
		if operationError == nil {
			operationError = operationContext.Err()
		}
		authenticationService.completeDeviceLogin(pendingLogin, credential, operationError)
		return
	}
}

func (authenticationService *Service) completeDeviceLogin(pendingLogin *loginFlow, credential atom.OAuthCredential, operationError error) {
	authenticationService.credentialGate <- struct{}{}
	defer authenticationService.releaseCredentialGate()
	if authenticationService.activeLogin != pendingLogin || pendingLogin.status.Status != "pending" || authenticationService.closed {
		return
	}
	if authenticationService.lifetimeContext.Err() != nil {
		operationError = authenticationService.lifetimeContext.Err()
	}
	if operationError == nil {
		operationError = authenticationService.secretStore.SaveOAuthCredential(authenticationService.lifetimeContext, pendingLogin.status.Provider, credential)
	}
	if operationError == nil {
		pendingLogin.status.Status = "connected"
		pendingLogin.status.UserCode = ""
		return
	}
	pendingLogin.status.Status = "error"
	pendingLogin.status.Error = operationError.Error()
	if errors.Is(operationError, context.DeadlineExceeded) {
		pendingLogin.status.Status = "expired"
		pendingLogin.status.Error = "The OpenAI device code expired. Start sign-in again."
	}
	if errors.Is(operationError, context.Canceled) {
		pendingLogin.status.Status = "cancelled"
	}
}
