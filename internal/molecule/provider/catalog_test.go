package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestCatalogMetadataPrecedenceAndUnknownCapabilities(test *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		fmt.Fprint(responseWriter, `{"data":[{"id":"configured-model","name":"Remote name","tools":false,"context_max":4096},{"id":"new-model"},{"id":"text-only","tools":false},{"id":"remote-model","tools":true,"context_max":8192}]}`)
	}))
	defer server.Close()
	var configuration Config
	testutil.RequireNoError(test, json.Unmarshal([]byte(`{"ModelDefaults":{"level":1},"Models":[{"id":"configured-model","name":"Configured name","tools":true,"context_max":16384}]}`), &configuration))
	standardProvider := New(atom.ProviderSpec{Name: "example", ModelListURL: server.URL})
	standardProvider.ConfigureModels(configuration.ModelDefaults, configuration.Models, []atom.ModelInfo{{ID: "configured-model", Name: "Stale name", Tools: false, ContextMax: 1}, {ID: "removed-model"}})
	models, operationError := standardProvider.Refresh(context.Background())
	testutil.RequireNoError(test, operationError)
	if len(models) != 4 {
		test.Fatalf("availability did not follow the remote catalog: %+v", models)
	}
	modelsByID := map[string]atom.ModelInfo{}
	for _, model := range models {
		modelsByID[model.ID] = model
	}
	configuredModel := modelsByID["configured-model"]
	if configuredModel.Name != "Configured name" || !configuredModel.Tools || configuredModel.ToolSupportUnknown || configuredModel.ContextMax != 16384 || configuredModel.Level != 1 {
		test.Fatalf("explicit configuration did not override cache/remote metadata: %+v", configuredModel)
	}
	if !modelsByID["new-model"].ToolSupportUnknown || modelsByID["new-model"].Tools {
		test.Fatal("missing metadata was represented as a known tool capability")
	}
	if modelsByID["text-only"].ToolSupportUnknown || modelsByID["text-only"].Tools {
		test.Fatal("explicit unsupported tools were lost")
	}
	if !modelsByID["remote-model"].Tools || modelsByID["remote-model"].ContextMax != 8192 {
		test.Fatal("remote capabilities were ignored")
	}
}

func TestExplicitModelConfigurationSurvivesCatalogChanges(test *testing.T) {
	catalogJSON := `{"data":[{"id":"remaining-model"}]}`
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		fmt.Fprint(responseWriter, catalogJSON)
	}))
	defer server.Close()
	standardProvider := New(atom.ProviderSpec{Name: "example", ModelListURL: server.URL})
	var configuration Config
	testutil.RequireNoError(test, json.Unmarshal([]byte(`{"ModelDefaults":{"tools":true},"Models":[{"id":"returning-model","name":"Configured model","tools":false,"level":0,"context_max":8192}]}`), &configuration))
	standardProvider.ConfigureModels(configuration.ModelDefaults, configuration.Models, nil)
	if len(standardProvider.Models()) != 0 {
		test.Fatal("metadata advertised an unfetched model")
	}
	models, operationError := standardProvider.Refresh(context.Background())
	testutil.RequireNoError(test, operationError)
	if len(models) != 1 || models[0].ID != "remaining-model" || !models[0].Tools {
		test.Fatal("configuration introduced an absent model or lost provider defaults")
	}
	catalogJSON = `{"data":[{"id":"returning-model","tools":true}]}`
	models, operationError = standardProvider.Refresh(context.Background())
	testutil.RequireNoError(test, operationError)
	if len(models) != 1 || models[0].ID != "returning-model" || models[0].Tools || models[0].ToolSupportUnknown || models[0].Name != "Configured model" || models[0].Level != 0 || models[0].ContextMax != 8192 {
		test.Fatalf("returning model lost explicit false/zero configuration: %+v", models)
	}
	catalogJSON = `{"data":[]}`
	models, operationError = standardProvider.Refresh(context.Background())
	testutil.RequireNoError(test, operationError)
	if len(models) != 0 {
		test.Fatal("empty successful catalog retained removed models")
	}
	standardProvider.ConfigureModels(configuration.ModelDefaults, configuration.Models, models)
	if len(standardProvider.Models()) != 0 {
		test.Fatal("startup reintroduced models removed by the provider")
	}
}

func TestMalformedCatalogKeepsLastUsableModels(test *testing.T) {
	for _, responseBody := range []string{`{}`, `{"data":null}`, `{"data":[{}]}`, `{"data":[{"id":"same"},{"id":"same"}]}`, `{"data":[]} invalid`, `{"data":[{"id":"invalid","context_max":-1}]}`} {
		test.Run(responseBody, func(test *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				fmt.Fprint(responseWriter, responseBody)
			}))
			defer server.Close()
			standardProvider := New(atom.ProviderSpec{Name: "example", ModelListURL: server.URL})
			previousModels := []atom.ModelInfo{{ID: "last-good", ToolSupportUnknown: true}}
			standardProvider.SetModels(previousModels)
			if _, operationError := standardProvider.Refresh(context.Background()); operationError == nil {
				test.Fatal("malformed catalog succeeded")
			}
			if !reflect.DeepEqual(standardProvider.Models(), previousModels) {
				test.Fatal("failed refresh replaced the last usable catalog")
			}
		})
	}
}

func TestCodexCatalogFetchesRemoteAvailabilityAndMetadata(test *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/models" || request.URL.Query().Get("client_version") != "0.101.0" || request.Header.Get("Authorization") != "Bearer test-access-token" || request.Header.Get("ChatGPT-Account-Id") != "test-account" {
			test.Error("catalog request lost configured URL or account authentication")
		}
		fmt.Fprint(responseWriter, `{"models":[{"slug":"remote-coding-model","display_name":"Remote coding model","context_window":32768,"input_modalities":["text","image"],"supports_parallel_tool_calls":false}]}`)
	}))
	defer server.Close()
	standardProvider := New(atom.ProviderSpec{Name: "example-coding-plan", ModelListURL: server.URL + "/models?client_version=0.101.0", ModelListFormat: "codex"})
	standardProvider.SetHeaderResolver(func(context.Context) (http.Header, error) {
		return http.Header{"Authorization": {"Bearer test-access-token"}, "Chatgpt-Account-Id": {"test-account"}}, nil
	})
	var defaults ModelMetadata
	testutil.RequireNoError(test, json.Unmarshal([]byte(`{"tools":true}`), &defaults))
	standardProvider.ConfigureModels(defaults, nil, []atom.ModelInfo{{ID: "removed-coding-model"}})
	models, operationError := standardProvider.Refresh(context.Background())
	testutil.RequireNoError(test, operationError)
	if len(models) != 1 || models[0].ID != "remote-coding-model" || models[0].Name != "Remote coding model" || models[0].ContextMax != 32768 || !models[0].Tools || !reflect.DeepEqual(models[0].Input, []atom.MediaType{atom.Text, atom.Image}) {
		test.Fatalf("Codex discovery did not use remote metadata: %+v", models)
	}
}
