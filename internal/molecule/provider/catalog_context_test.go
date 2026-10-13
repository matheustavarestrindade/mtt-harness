package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestCodexContextUsesAdvertisedMaximumWithLegacyFallback(test *testing.T) {
	for _, testCase := range []struct {
		name       string
		fields     string
		contextMax int
	}{
		{"maximum overrides client default", `"context_window":32000,"max_context_window":192000`, 192000},
		{"maximum without client default", `"max_context_window":256000`, 256000},
		{"legacy default", `"context_window":64000`, 64000},
		{"null maximum", `"context_window":48000,"max_context_window":null`, 48000},
		{"unknown zero maximum", `"context_window":48000,"max_context_window":0`, 48000},
		{"maximum can constrain client default", `"context_window":192000,"max_context_window":64000`, 64000},
	} {
		test.Run(testCase.name, func(test *testing.T) {
			catalog, operationError := decodeModelCatalog(strings.NewReader(`{"models":[{"slug":"synthetic-model",`+testCase.fields+`}]}`), "codex")
			testutil.RequireNoError(test, operationError)
			if len(catalog) != 1 || catalog[0].ContextMax == nil || *catalog[0].ContextMax != testCase.contextMax {
				test.Fatalf("wrong context metadata: %+v", catalog)
			}
		})
	}
}

func TestCodexMaximumRefreshReplacesCachedLimitAndReachesTierPresets(test *testing.T) {
	responseBody := `{"models":[{"slug":"synthetic-model","context_window":32000,"max_context_window":192000,"service_tiers":[{"id":"quick","name":"Quick"}]},{"slug":"configured-model","context_window":32000,"max_context_window":192000}]}`
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		fmt.Fprint(responseWriter, responseBody)
	}))
	defer server.Close()
	standardProvider := New(atom.ProviderSpec{Name: "catalog-fixture", ModelListURL: server.URL, ModelListFormat: "codex"})
	override := 96000
	standardProvider.ConfigureModels(ModelMetadata{}, []ModelConfiguration{{ID: "configured-model", ModelMetadata: ModelMetadata{ContextMax: &override}}}, []atom.ModelInfo{{ID: "synthetic-model", ContextMax: 32000}})
	models, operationError := standardProvider.Refresh(context.Background())
	testutil.RequireNoError(test, operationError)
	if len(models) != 3 || models[0].ContextMax != 192000 || models[1].ContextMax != 192000 || models[1].APIModel != "synthetic-model" || models[1].ServiceTier != "quick" || models[2].ContextMax != override {
		test.Fatalf("refresh lost maximum, tier inheritance, or explicit override: %+v", models)
	}
	responseBody = `{"models":[{"slug":"synthetic-model","context_window":32000,"max_context_window":128000}]}`
	models, operationError = standardProvider.Refresh(context.Background())
	testutil.RequireNoError(test, operationError)
	if len(models) != 1 || models[0].ContextMax != 128000 {
		test.Fatalf("changed maximum did not replace cached metadata: %+v", models)
	}
	responseBody = `{"models":[{"slug":"synthetic-model","context_window":32000,"max_context_window":-1}]}`
	if _, operationError := standardProvider.Refresh(context.Background()); operationError == nil {
		test.Fatal("negative maximum was accepted")
	}
	if !reflect.DeepEqual(standardProvider.Models(), models) {
		test.Fatal("malformed maximum discarded the last valid catalog")
	}
}
