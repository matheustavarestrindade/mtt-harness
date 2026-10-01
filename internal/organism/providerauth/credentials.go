package providerauth

import (
	"context"
	"errors"
	"net/http"
	"time"
)

func (authenticationService *Service) DisconnectProvider(operationContext context.Context, providerID string) error {
	if operationError := authenticationService.acquireCredentialGate(operationContext); operationError != nil {
		return operationError
	}
	defer authenticationService.releaseCredentialGate()
	if authenticationService.activeLogin != nil && authenticationService.activeLogin.status.Provider == providerID {
		authenticationService.activeLogin.cancel()
		authenticationService.activeLogin.status.Status = "cancelled"
	}
	return authenticationService.secretStore.DeleteOAuthCredential(operationContext, providerID)
}

// ProviderAuthenticationHeaders resolves persisted tokens and serializes refresh
// so concurrent model/catalog requests cannot rotate the same token twice.
func (authenticationService *Service) ProviderAuthenticationHeaders(operationContext context.Context, providerID string) (http.Header, error) {
	if operationError := authenticationService.acquireCredentialGate(operationContext); operationError != nil {
		return nil, operationError
	}
	defer authenticationService.releaseCredentialGate()
	credential, operationError := authenticationService.secretStore.OAuthCredential(operationContext, providerID)
	if operationError != nil {
		return nil, operationError
	}
	if credential.AccessToken == "" {
		return nil, errors.New("connect the OpenAI coding plan in Providers")
	}
	if time.Until(credential.ExpiresAt) < time.Minute {
		credential, operationError = authenticationService.authenticationClient.RefreshAccessToken(operationContext, credential)
		if operationError != nil {
			return nil, operationError
		}
		if operationError := authenticationService.secretStore.SaveOAuthCredential(operationContext, providerID, credential); operationError != nil {
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
