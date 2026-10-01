package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/api"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestProviderSetupListsActiveProvidersWithoutSecrets(test *testing.T) {
	database := memory.New()
	modelGateway := gateway.New(harness.New())
	for _, specification := range []atom.ProviderSpec{
		{Name: "configured-api", Authentication: "api_key"},
		{Name: "other-api", Authentication: "api_key"},
		{Name: "example-coding-plan", Authentication: "chatgpt"},
	} {
		registered := provider.New(specification)
		testutil.RequireNoError(test, modelGateway.Add(registered))
	}
	testutil.RequireNoError(test, modelGateway.Add(provider.NewTest("test")))
	testutil.RequireNoError(test, database.Providers().Save(context.Background(), atom.ProviderSpec{Name: "stale-provider"}))
	server := httptest.NewServer(api.New(api.Config{Token: "api-token", Store: database, Gateway: modelGateway}).Handler())
	defer server.Close()
	response := request(test, server.URL+"/providers", "api-token", "GET", nil)
	var providers []atom.ProviderConnection
	decode(test, response, &providers)
	if len(providers) != 4 {
		test.Fatalf("registered provider count = %d", len(providers))
	}
	for _, connection := range providers {
		if connection.Name == "stale-provider" {
			test.Fatal("stale database provider was advertised")
		}
		if connection.Connected != (connection.Name == "test") {
			test.Fatalf("initial connection = %#v", connection)
		}
	}
	response = request(test, server.URL+"/providers/configured-api/key", "api-token", "PUT", map[string]string{"key": "example-key"})
	if response.StatusCode != http.StatusOK {
		test.Fatalf("save key = %d", response.StatusCode)
	}
	response.Body.Close()
	response = request(test, server.URL+"/providers", "api-token", "GET", nil)
	decode(test, response, &providers)
	for _, connection := range providers {
		if connection.Name == "configured-api" && !connection.Connected {
			test.Fatal("saved key was not reflected in status")
		}
		if strings.Contains(connection.APIURL, "example-key") {
			test.Fatal("key leaked into provider data")
		}
	}
	response = request(test, server.URL+"/providers/test/models", "api-token", "GET", nil)
	var models []atom.ModelInfo
	decode(test, response, &models)
	if len(models) == 0 || !models[0].Tools {
		test.Fatal("test provider models are absent from the public catalog")
	}
	response = request(test, server.URL+"/providers/example-coding-plan/key", "api-token", "PUT", map[string]string{"key": "wrong-auth-method"})
	if response.StatusCode != http.StatusBadRequest {
		test.Fatal("coding plan accepted an API key")
	}
	response.Body.Close()
	response = request(test, server.URL+"/providers/example-coding-plan/auth/device", "", "POST", nil)
	if response.StatusCode != http.StatusUnauthorized {
		test.Fatal("device login bypassed harness authentication")
	}
	response.Body.Close()
}
