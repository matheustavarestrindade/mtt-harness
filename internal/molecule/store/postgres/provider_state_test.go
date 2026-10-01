package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestProviderAuthenticationAndReasoningPersistence(test *testing.T) {
	database := regressionStore(test)
	operationContext := context.Background()
	identifier := fmt.Sprintf("provider-state-%d", time.Now().UnixNano())
	credential := atom.OAuthCredential{AccessToken: "example-access", RefreshToken: "example-refresh", AccountID: "example-account", Residency: "us", ExpiresAt: time.Now().Add(time.Hour).Truncate(time.Microsecond)}
	testutil.RequireNoError(test, database.Secrets().SaveOAuthCredential(operationContext, identifier, credential))
	stored, operationError := database.Secrets().OAuthCredential(operationContext, identifier)
	testutil.RequireNoError(test, operationError)
	if stored.AccessToken != credential.AccessToken || stored.RefreshToken != credential.RefreshToken || !stored.ExpiresAt.Equal(credential.ExpiresAt) || stored.AccountID != credential.AccountID || stored.Residency != credential.Residency {
		test.Fatal("OAuth credentials did not survive database round trip")
	}
	specification := atom.ProviderSpec{Name: identifier, Protocol: "responses", Authentication: "chatgpt", APIURL: "https://example.invalid", ModelListFormat: "codex", Interval: time.Hour}
	testutil.RequireNoError(test, database.Providers().Save(operationContext, specification))
	storedSpec, operationError := database.Providers().Get(operationContext, identifier)
	testutil.RequireNoError(test, operationError)
	if storedSpec.Protocol != "responses" || storedSpec.Authentication != "chatgpt" || storedSpec.ModelListFormat != "codex" {
		test.Fatalf("provider metadata = %#v", storedSpec)
	}
	model := atom.ModelInfo{ID: "unclassified-model", Name: "Example model", ToolSupportUnknown: true, ContextMax: 8192}
	testutil.RequireNoError(test, database.Providers().SaveModels(operationContext, identifier, []atom.ModelInfo{model}))
	storedModels, operationError := database.Providers().Models(operationContext, identifier)
	testutil.RequireNoError(test, operationError)
	if len(storedModels) != 1 || storedModels[0].ID != model.ID || storedModels[0].Name != model.Name || !storedModels[0].ToolSupportUnknown || storedModels[0].Tools {
		test.Fatalf("model names or capabilities were lost in storage: %+v", storedModels)
	}
	session := atom.Session{ID: atom.SessionID(identifier), InstanceID: identifier, CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	message := atom.Message{ID: identifier, SessionID: session.ID, Role: atom.RoleAssistant, CreatedAt: time.Now(), ProviderState: &atom.ProviderState{Provider: identifier, Reasoning: []json.RawMessage{json.RawMessage(`{"type":"reasoning","encrypted_content":"opaque"}`)}}}
	testutil.RequireNoError(test, database.Sessions().Append(operationContext, message))
	messages, operationError := database.Sessions().Messages(operationContext, session.ID)
	testutil.RequireNoError(test, operationError)
	if len(messages) != 1 || messages[0].ProviderState == nil || messages[0].ProviderState.Provider != identifier || len(messages[0].ProviderState.Reasoning) != 1 {
		test.Fatalf("reasoning continuation was lost: %#v", messages)
	}
	var original, restored any
	testutil.RequireNoError(test, json.Unmarshal(message.ProviderState.Reasoning[0], &original))
	testutil.RequireNoError(test, json.Unmarshal(messages[0].ProviderState.Reasoning[0], &restored))
	originalJSON, _ := json.Marshal(original)
	restoredJSON, _ := json.Marshal(restored)
	if string(originalJSON) != string(restoredJSON) {
		test.Fatal("reasoning JSON changed after storage")
	}
	testutil.RequireNoError(test, database.Secrets().DeleteOAuthCredential(operationContext, identifier))
	stored, operationError = database.Secrets().OAuthCredential(operationContext, identifier)
	testutil.RequireNoError(test, operationError)
	if stored.AccessToken != "" || stored.RefreshToken != "" {
		test.Fatal("disconnect left OAuth tokens in the database")
	}
}
