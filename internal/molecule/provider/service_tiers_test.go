package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestCodexCatalogVisibilityTiersAndRefresh(test *testing.T) {
	catalog := `{"models":[
		{"slug":"internal-review","display_name":"Internal","visibility":"hide"},
		{"slug":"withdrawn","visibility":"none"},
		{"slug":"coding-example","display_name":"Example coding","visibility":"list","context_window":64000,"input_modalities":["text","image"],"supported_reasoning_levels":[{"effort":"low"},{"effort":"future-effort"}],"default_reasoning_level":"low","supports_reasoning_summaries":true,"supports_reasoning_summary_parameter":false,"service_tiers":[{"id":"expedited","name":"Quick"}]},
		{"slug":"legacy-example","display_name":"Legacy"}
	]}`
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		fmt.Fprint(responseWriter, catalog)
	}))
	defer server.Close()
	standardProvider := New(atom.ProviderSpec{Name: "coding-provider", ModelListURL: server.URL, ModelListFormat: "codex"})
	var defaults ModelMetadata
	testutil.RequireNoError(test, json.Unmarshal([]byte(`{"tools":true,"level":2,"reasoning_summary":"auto"}`), &defaults))
	standardProvider.ConfigureModels(defaults, nil, nil)
	standardProvider.SetPrices(map[string]atom.Prices{"coding-example": {Currency: "USD", Input: 3, Output: 7}})
	models, operationError := standardProvider.Refresh(context.Background())
	testutil.RequireNoError(test, operationError)
	if len(models) != 3 || models[0].ID != "coding-example" || models[1].ID != "coding-example@expedited" || models[2].ID != "legacy-example" {
		test.Fatalf("visible catalog and tier selections = %+v", models)
	}
	preset := models[1]
	if preset.APIModel != "coding-example" || preset.ServiceTier != "expedited" || preset.Name != "Example coding (Quick)" || preset.ContextMax != 64000 || !preset.Tools || preset.Level != 2 || preset.ReasoningSummary != "" || !reflect.DeepEqual(preset.ReasoningEfforts, []string{"low", "future-effort"}) {
		test.Fatalf("preset metadata = %+v", preset)
	}
	if models[0].Prices == nil || preset.Prices != nil {
		test.Fatal("preset inherited base-model token prices")
	}
	standardProvider.SetPrices(map[string]atom.Prices{"coding-example@expedited": {Currency: "USD", Input: 5, Output: 9}})
	models, operationError = standardProvider.Refresh(context.Background())
	testutil.RequireNoError(test, operationError)
	if models[1].Prices == nil || models[1].Prices.Input != 5 {
		test.Fatal("explicit preset prices were lost")
	}
	// A disappearing tier cannot remain available from the saved catalog.
	catalog = `{"models":[{"slug":"coding-example","display_name":"Updated","visibility":"list","service_tiers":[]}]}`
	models, operationError = standardProvider.Refresh(context.Background())
	testutil.RequireNoError(test, operationError)
	if len(models) != 1 || models[0].APIModel != "" || models[0].ServiceTier != "" {
		test.Fatalf("stale preset survived: %+v", models)
	}
	catalog = `{"models":[]}`
	models, operationError = standardProvider.Refresh(context.Background())
	testutil.RequireNoError(test, operationError)
	if len(models) != 0 {
		test.Fatalf("empty remote catalog kept selections: %+v", models)
	}
}

func TestCodexMalformedTiersKeepLastCatalog(test *testing.T) {
	for _, catalog := range []string{
		`{"models":[{"slug":"model-a","service_tiers":[{"id":"","name":"Empty"}]}]}`,
		`{"models":[{"slug":"model-a","service_tiers":[{"id":"quick"},{"id":"quick"}]}]}`,
		`{"models":[{"slug":"model-a","service_tiers":[{"id":"quick"}]},{"slug":"model-a@quick"}]}`,
	} {
		test.Run(catalog, func(test *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) { fmt.Fprint(responseWriter, catalog) }))
			defer server.Close()
			standardProvider := New(atom.ProviderSpec{Name: "example", ModelListURL: server.URL, ModelListFormat: "codex"})
			standardProvider.SetModels([]atom.ModelInfo{{ID: "last-good"}})
			if _, operationError := standardProvider.Refresh(context.Background()); operationError == nil {
				test.Fatal("invalid catalog was accepted")
			}
			if models := standardProvider.Models(); len(models) != 1 || models[0].ID != "last-good" {
				test.Fatalf("last good catalog was replaced: %+v", models)
			}
		})
	}
}

func TestCatalogPresetUsesOriginalModelAndTierOnWire(test *testing.T) {
	for _, protocol := range []string{"responses", "chat_completions"} {
		test.Run(protocol, func(test *testing.T) {
			requests := make(chan map[string]any, 2)
			server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/models" {
					fmt.Fprint(responseWriter, `{"models":[{"slug":"remote-model","display_name":"Remote","visibility":"list","context_window":64000,"service_tiers":[{"id":"expedited","name":"Quick"}],"supported_reasoning_levels":[{"effort":"low"}],"default_reasoning_level":"low"}]}`)
					return
				}
				var payload map[string]any
				if operationError := json.NewDecoder(request.Body).Decode(&payload); operationError != nil {
					test.Error(operationError)
					http.Error(responseWriter, "invalid body", 400)
					return
				}
				requests <- payload
				responseWriter.Header().Set("Content-Type", "text/event-stream")
				if protocol == "responses" {
					fmt.Fprint(responseWriter, "data: {\"type\":\"response.completed\",\"response\":{\"output\":[]}}\n\n")
					return
				}
				fmt.Fprint(responseWriter, "data: [DONE]\n\n")
			}))
			defer server.Close()
			standardProvider := New(atom.ProviderSpec{Name: "example-coding", Protocol: protocol, APIURL: server.URL, ModelListURL: server.URL + "/models", ModelListFormat: "codex", Authentication: "chatgpt", Billing: "subscription"})
			models, operationError := standardProvider.Refresh(context.Background())
			testutil.RequireNoError(test, operationError)
			// Cache reload must preserve transport mapping independently of discovery.
			serialized, operationError := json.Marshal(models)
			testutil.RequireNoError(test, operationError)
			var restored []atom.ModelInfo
			testutil.RequireNoError(test, json.Unmarshal(serialized, &restored))
			standardProvider.ConfigureModels(ModelMetadata{}, nil, restored)
			for _, selection := range []string{"remote-model@expedited", "remote-model"} {
				request := atom.Request{Model: selection, Messages: []atom.Message{{Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "hello"}}}}, ReasoningEffort: "low", Params: map[string]any{"max_output_tokens": 128}}
				stream, operationError := standardProvider.Stream(context.Background(), request)
				testutil.RequireNoError(test, operationError)
				for {
					_, operationError = stream.Recv(context.Background())
					if errors.Is(operationError, io.EOF) {
						break
					}
					testutil.RequireNoError(test, operationError)
				}
				payload := <-requests
				if payload["model"] != "remote-model" {
					test.Fatalf("preset ID leaked onto wire: %+v", payload)
				}
				if selection == "remote-model@expedited" && payload["service_tier"] != "expedited" {
					test.Fatalf("tier was not sent: %+v", payload)
				}
				if selection == "remote-model" && payload["service_tier"] != nil {
					test.Fatal("tier contaminated a later base-model request")
				}
				if protocol == "responses" && payload["max_output_tokens"] != nil {
					test.Fatal("coding-plan request included unsupported output cap")
				}
				if request.Model != selection || len(request.Params) != 1 {
					test.Fatal("provider mutated the harness request")
				}
			}
			_, operationError = standardProvider.Stream(context.Background(), atom.Request{Model: "remote-model@expedited", Params: map[string]any{"service_tier": "other"}})
			if operationError == nil || !strings.Contains(operationError.Error(), "requires service tier") {
				test.Fatalf("conflicting tier = %v", operationError)
			}
		})
	}
}
