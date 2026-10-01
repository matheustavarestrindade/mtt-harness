package openaiauth

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

// RefreshAccessToken rotates the credential using its existing refresh token.
func (authenticationClient *Client) RefreshAccessToken(operationContext context.Context, credential atom.OAuthCredential) (atom.OAuthCredential, error) {
	if credential.RefreshToken == "" {
		return atom.OAuthCredential{}, errors.New("sign in with ChatGPT again; no refresh token is available")
	}
	return authenticationClient.requestOAuthTokens(operationContext, url.Values{"grant_type": {"refresh_token"}, "client_id": {OpenAICodexOAuthClientID}, "refresh_token": {credential.RefreshToken}}, credential)
}

func (authenticationClient *Client) requestOAuthTokens(operationContext context.Context, tokenRequestParameters url.Values, previousCredential atom.OAuthCredential) (atom.OAuthCredential, error) {
	var response struct {
		AccessToken    string `json:"access_token"`
		RefreshToken   string `json:"refresh_token"`
		IDToken        string `json:"id_token"`
		ExpiresSeconds int64  `json:"expires_in"`
	}
	_, operationError := authenticationClient.sendAuthenticationRequest(operationContext, "/oauth/token", "application/x-www-form-urlencoded", []byte(tokenRequestParameters.Encode()), &response)
	if operationError != nil {
		return atom.OAuthCredential{}, operationError
	}
	if response.AccessToken == "" {
		return atom.OAuthCredential{}, errors.New("OpenAI returned no access token")
	}
	credential := previousCredential
	credential.AccessToken = response.AccessToken
	if response.RefreshToken != "" {
		credential.RefreshToken = response.RefreshToken
	}
	if credential.RefreshToken == "" {
		return atom.OAuthCredential{}, errors.New("OpenAI returned no refresh token")
	}
	if response.ExpiresSeconds <= 0 {
		response.ExpiresSeconds = 3600
	}
	if response.ExpiresSeconds > 365*24*3600 {
		return atom.OAuthCredential{}, errors.New("OpenAI returned an invalid token expiry")
	}
	credential.ExpiresAt = time.Now().Add(time.Duration(response.ExpiresSeconds) * time.Second)
	for _, token := range []string{response.IDToken, response.AccessToken} {
		accountID, computeResidency := extractChatGPTAccountRouting(token)
		if accountID != "" {
			credential.AccountID = accountID
		}
		if computeResidency != "" {
			credential.Residency = computeResidency
		}
	}
	if credential.AccountID == "" {
		return atom.OAuthCredential{}, errors.New("OpenAI returned no ChatGPT account ID")
	}
	return credential, nil
}
