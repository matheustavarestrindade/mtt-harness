package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestProviderTransportSurvivesPostgresRoundTrip(test *testing.T) {
	database := regressionStore(test)
	operationContext := context.Background()
	specification := atom.ProviderSpec{Name: fmt.Sprintf("transport-%d", time.Now().UnixNano()), Protocol: "responses", APIURL: "https://api.example/v1", Transport: "auto", WebSocketURL: "wss://socket.example/responses"}
	testutil.RequireNoError(test, database.Providers().Save(operationContext, specification))
	test.Cleanup(func() {
		testutil.RequireNoError(test, database.Providers().Delete(operationContext, specification.Name))
	})
	loaded, operationError := database.Providers().Get(operationContext, specification.Name)
	testutil.RequireNoError(test, operationError)
	if loaded.Transport != specification.Transport || loaded.WebSocketURL != specification.WebSocketURL {
		test.Fatalf("transport configuration was lost: %#v", loaded)
	}
	all, operationError := database.Providers().All(operationContext)
	testutil.RequireNoError(test, operationError)
	found := false
	for _, provider := range all {
		if provider.Name == specification.Name {
			found = true
			if provider.Transport != specification.Transport || provider.WebSocketURL != specification.WebSocketURL {
				test.Fatal("provider listing lost transport metadata")
			}
		}
	}
	if !found {
		test.Fatal("saved provider missing from catalog")
	}
	specification.Transport = "http"
	specification.WebSocketURL = ""
	testutil.RequireNoError(test, database.Providers().Save(operationContext, specification))
	loaded, operationError = database.Providers().Get(operationContext, specification.Name)
	testutil.RequireNoError(test, operationError)
	if loaded.Transport != "http" || loaded.WebSocketURL != "" {
		test.Fatal("transport override was not refreshed")
	}
}
