package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/providerauth"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestDefaultProvidersCoexistWithTestProvider(test *testing.T) {
	operationContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	database := memory.New()
	modelGateway := gateway.New(harness.New())
	testutil.RequireNoError(test, modelGateway.Add(provider.NewTest("test")))
	authentication := providerauth.New(operationContext, database.Secrets(), nil)
	defer authentication.Close()
	loadProviders(operationContext, filepath.Join(test.TempDir(), "not-created.json"), modelGateway, database, authentication)
	for _, name := range []string{"test", "openai", "deepseek", provider.CodexProvider} {
		registered, found := modelGateway.Provider(name)
		if !found || len(registered.Models()) == 0 {
			test.Fatalf("provider %s was not loaded with its defaults", name)
		}
	}
}
