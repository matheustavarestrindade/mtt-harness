package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

const metadataFixture = `{"catalog-provider":{"models":{
	"model-current":{"id":"model-current","name":"Catalog name","reasoning":true,"reasoning_options":[{"type":"toggle"},{"type":"effort","values":["low","high"]}],"tool_call":true,"limit":{"context":5000},"modalities":{"input":["text","image"],"output":["text"]},"cost":{"input":1,"output":4,"cache_read":0.2,"tiers":[{"tier":{"type":"context","size":100},"input":2,"output":6,"cache_read":0.4}]}},
	"source-only":{"id":"source-only","reasoning":false,"cost":{"input":0,"output":0}},
	"unpriced":{"id":"unpriced","cost":{"output":3}}
}}}`

func TestMetadataEnrichesOnlyProviderAvailabilityAndPreservesOverrides(test *testing.T) {
	availability := `{"data":[{"id":"model-current","context_max":6000},{"id":"remote-only"},{"id":"unpriced"}]}`
	metadata := metadataFixture
	var sourceMutex sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		sourceMutex.Lock()
		defer sourceMutex.Unlock()
		if request.URL.Path == "/metadata" {
			if request.Header.Get("Authorization") != "" || request.Header.Get("User-Agent") != "mtt-harness/1.0" {
				test.Error("public metadata received provider credentials or lost its user agent")
			}
			fmt.Fprint(responseWriter, metadata)
			return
		}
		if request.Header.Get("Authorization") != "Bearer private-key" {
			test.Error("provider catalog lost authentication")
		}
		fmt.Fprint(responseWriter, availability)
	}))
	defer server.Close()
	standardProvider := New(atom.ProviderSpec{Name: "configured-provider", Protocol: "responses", ModelListURL: server.URL + "/models", MetadataURL: server.URL + "/metadata", MetadataFormat: "models_dev", MetadataProvider: "catalog-provider"})
	standardProvider.SetKeyResolver(func(context.Context, string, string) (string, error) { return "private-key", nil })
	name, contextLimit := "Local name", 7000
	standardProvider.ConfigureModels(ModelMetadata{}, []ModelConfiguration{{ID: "model-current", ModelMetadata: ModelMetadata{Name: &name, ContextMax: &contextLimit}}}, nil)
	models, operationError := standardProvider.Refresh(context.Background())
	testutil.RequireNoError(test, operationError)
	if len(models) != 3 || models[0].Name != name || models[0].ContextMax != contextLimit || !models[0].Reasoning || strings.Join(models[0].ReasoningEfforts, ",") != "none,low,high" {
		test.Fatalf("metadata/availability precedence is wrong: %+v", models)
	}
	if models[0].Prices == nil || models[0].Prices.Input != 1 || len(models[0].Prices.Tiers) != 1 || !models[0].Prices.CacheWriteUnknown {
		test.Fatalf("rates, context tiers or unknown cache pricing were lost: %+v", models[0].Prices)
	}
	if models[1].Prices != nil || models[2].Prices != nil {
		test.Fatal("missing metadata became a free price")
	}
	standardProvider.SetPrices(map[string]atom.Prices{"model-current": {Currency: "EUR", Input: 8, Output: 9}})
	models, operationError = standardProvider.Refresh(context.Background())
	testutil.RequireNoError(test, operationError)
	if models[0].Prices.Currency != "EUR" || models[0].Prices.Input != 8 {
		test.Fatal("catalog overwrote explicit configuration prices")
	}
	sourceMutex.Lock()
	metadata = `{"invalid":{}}`
	sourceMutex.Unlock()
	if _, operationError := standardProvider.Refresh(context.Background()); operationError == nil {
		test.Fatal("invalid metadata was accepted")
	}
	if len(standardProvider.Models()) != 3 || standardProvider.Models()[0].Prices.Input != 8 {
		test.Fatal("failed refresh discarded the last good catalog")
	}
	sourceMutex.Lock()
	metadata, availability = metadataFixture, `{"data":[]}`
	sourceMutex.Unlock()
	models, operationError = standardProvider.Refresh(context.Background())
	testutil.RequireNoError(test, operationError)
	if len(models) != 0 {
		test.Fatal("metadata resurrected remotely removed models")
	}
}

func TestMetadataKeepsExplicitZeroAndRejectsInvalidRates(test *testing.T) {
	models, operationError := decodeModelsDevMetadata([]byte(metadataFixture), "catalog-provider", "source", true)
	testutil.RequireNoError(test, operationError)
	if models["source-only"].Prices == nil || models["source-only"].Prices.Input != 0 || models["unpriced"].Prices != nil {
		test.Fatal("zero prices and absent prices were conflated")
	}
	for _, payload := range []string{
		strings.Replace(metadataFixture, `"input":1`, `"input":-1`, 1),
		strings.Replace(metadataFixture, `"model-current":{"id":"model-current"`, `"model-current":{"id":"mismatch"`, 1),
	} {
		if _, operationError := decodeModelsDevMetadata([]byte(payload), "catalog-provider", "source", true); operationError == nil {
			test.Fatal("invalid metadata was accepted")
		}
	}
	standardProvider := New(atom.ProviderSpec{Billing: "subscription"})
	standardProvider.SetPrices(map[string]atom.Prices{"model-current": {Input: 99, Currency: "USD"}})
	standardProvider.SetModels([]atom.ModelInfo{{ID: "model-current", Prices: models["model-current"].Prices}})
	standardProvider.SetPrices(map[string]atom.Prices{"model-current": {Input: 199, Currency: "USD"}})
	if standardProvider.Models()[0].Prices != nil || standardProvider.Models()[0].Billing != "subscription" {
		test.Fatal("subscription access inherited API token charges")
	}
}

func TestMetadataSourceAcceptsFullCatalogSizeAndRejectsTruncation(test *testing.T) {
	path := filepath.Join(test.TempDir(), "metadata.json")
	testutil.RequireNoError(test, os.WriteFile(path, []byte(metadataFixture+strings.Repeat(" ", 2*1024*1024)), 0o600))
	standardProvider := New(atom.ProviderSpec{MetadataURL: path, MetadataFormat: "models_dev", MetadataProvider: "catalog-provider"})
	models, operationError := standardProvider.loadModelMetadata(context.Background())
	testutil.RequireNoError(test, operationError)
	if len(models) != 3 {
		test.Fatal("metadata source was limited to the old 1 MiB price-table limit")
	}
	if _, operationError := standardProvider.readCatalogSource(context.Background(), path, 100); operationError == nil || !strings.Contains(operationError.Error(), "exceeds") {
		test.Fatal("oversized source was silently truncated")
	}
}

func TestModelsDevLiveCatalog(test *testing.T) {
	if os.Getenv("MTT_TEST_LIVE_MODEL_METADATA") != "1" {
		test.Skip("set MTT_TEST_LIVE_MODEL_METADATA=1 for live catalog verification")
	}
	configurations, operationError := LoadFile("../../../providers.json")
	testutil.RequireNoError(test, operationError)
	operationContext, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	for _, configuration := range configurations {
		if configuration.Spec.MetadataURL == "" {
			continue
		}
		models, operationError := New(configuration.Spec).loadModelMetadata(operationContext)
		testutil.RequireNoError(test, operationError)
		priced, reasoning := 0, 0
		for _, model := range models {
			if model.Prices != nil {
				priced++
			}
			if model.Reasoning != nil && *model.Reasoning {
				reasoning++
			}
			_, operationError := json.Marshal(model)
			testutil.RequireNoError(test, operationError)
		}
		if priced == 0 || reasoning == 0 {
			test.Fatalf("live catalog lost expected metadata: %d prices, %d reasoning models", priced, reasoning)
		}
		test.Logf("%s: %d models, %d prices, %d reasoning models", configuration.Spec.MetadataProvider, len(models), priced, reasoning)
	}
}
