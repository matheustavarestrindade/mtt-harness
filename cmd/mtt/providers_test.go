package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/providerauth"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestConfiguredProvidersCoexistWithTestProvider(test *testing.T) {
	operationContext, cancelApplication := context.WithCancel(context.Background())
	defer cancelApplication()
	database := memory.New()
	modelGateway := gateway.New(harness.New())
	testutil.RequireNoError(test, modelGateway.Add(provider.NewTest("test")))
	authentication := providerauth.New(operationContext, database.Secrets(), nil)
	defer authentication.Close()
	configurationPath := filepath.Join(test.TempDir(), "providers.json")
	configurationJSON := `{"providers":[{"name":"custom-provider","api_url":"https://example.invalid","authentication":"none","models":[{"id":"configured-model","tools":true}]}]}`
	testutil.RequireNoError(test, os.WriteFile(configurationPath, []byte(configurationJSON), 0600))
	loadProviders(operationContext, configurationPath, modelGateway, database, authentication)
	if len(modelGateway.Providers()) != 2 {
		test.Fatal("provider loading injected providers absent from JSON")
	}
	registeredProvider, found := modelGateway.Provider("custom-provider")
	if !found || len(registeredProvider.Models()) != 1 || registeredProvider.Models()[0].ID != "configured-model" {
		test.Fatal("configured provider/model was not loaded")
	}
	if _, found := modelGateway.Provider("test"); !found {
		test.Fatal("file providers replaced the test provider")
	}
}

func TestMissingProviderFileDoesNotInjectDefaults(test *testing.T) {
	database := memory.New()
	modelGateway := gateway.New(harness.New())
	loadProviders(context.Background(), filepath.Join(test.TempDir(), "missing.json"), modelGateway, database, nil)
	if len(modelGateway.Providers()) != 0 {
		test.Fatal("missing JSON file injected hard-coded providers")
	}
}
