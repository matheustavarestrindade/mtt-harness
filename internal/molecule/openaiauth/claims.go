package openaiauth

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// Claims are read only from tokens returned by the HTTPS OAuth exchange. They
// provide routing metadata, not an independent authentication decision.
func extractChatGPTAccountRouting(token string) (string, string) {
	tokenParts := strings.Split(token, ".")
	if len(tokenParts) != 3 {
		return "", ""
	}
	claimsJSON, operationError := base64.RawURLEncoding.DecodeString(tokenParts[1])
	if operationError != nil {
		return "", ""
	}
	type accountRouting struct {
		AccountID string `json:"chatgpt_account_id"`
		Residency string `json:"chatgpt_compute_residency"`
	}
	var tokenClaims struct {
		accountRouting
		Authentication accountRouting `json:"https://api.openai.com/auth"`
		Organizations  []struct {
			ID string `json:"id"`
		} `json:"organizations"`
	}
	if json.Unmarshal(claimsJSON, &tokenClaims) != nil {
		return "", ""
	}
	accountID, computeResidency := tokenClaims.AccountID, tokenClaims.Residency
	if accountID == "" {
		accountID = tokenClaims.Authentication.AccountID
	}
	if computeResidency == "" {
		computeResidency = tokenClaims.Authentication.Residency
	}
	if accountID == "" && len(tokenClaims.Organizations) > 0 {
		accountID = tokenClaims.Organizations[0].ID
	}
	if computeResidency == "no_constraint" {
		computeResidency = ""
	}
	return accountID, computeResidency
}
