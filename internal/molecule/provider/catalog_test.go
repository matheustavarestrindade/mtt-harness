package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestDeepSeekRefreshRepairsStaleModelMetadata(test *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(responseWriter).Encode(map[string]any{
			"data": []map[string]string{{"id": "deepseek-flash"}, {"id": "deepseek-v4-pro"}, {"id": "future-model"}},
		})
	}))
	defer server.Close()
	standardProvider := New(atom.ProviderSpec{Name: "deepseek", ModelListURL: server.URL})
	standardProvider.SetModels([]atom.ModelInfo{
		{ID: "deepseek-flash", Name: "Old Flash", Input: []atom.MediaType{atom.Text}},
		{ID: "deepseek-v4-pro", Tools: false, ContextMax: 0},
	})
	for refreshCount := 0; refreshCount < 2; refreshCount++ {
		models, operationError := standardProvider.Refresh(context.Background())
		testutil.RequireNoError(test, operationError)
		if len(models) != 3 {
			test.Fatalf("provider model IDs were dropped: %+v", models)
		}
		byID := map[string]atom.ModelInfo{}
		for _, model := range models {
			byID[model.ID] = model
		}
		flashModel := byID["deepseek-flash"]
		if flashModel.Name != "DeepSeek-V4.1-Flash" || !flashModel.Tools || flashModel.ContextMax != 1000000 || !reflect.DeepEqual(flashModel.Input, []atom.MediaType{atom.Text, atom.Image}) {
			test.Fatalf("Flash metadata disagrees with the current DeepSeek contract: %+v", flashModel)
		}
		proModel := byID["deepseek-v4-pro"]
		if proModel.Name != "DeepSeek-V4-Pro-0813" || !proModel.Tools || proModel.ContextMax != 1000000 || !reflect.DeepEqual(proModel.Input, []atom.MediaType{atom.Text}) {
			test.Fatalf("Pro metadata disagrees with the current DeepSeek contract: %+v", proModel)
		}
		if byID["future-model"].Tools {
			test.Fatal("tool support was invented for an unknown model")
		}
	}
}

func TestExplicitModelConfigurationSurvivesCatalogChanges(test *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		models := []map[string]string{{"id": "deepseek-flash"}}
		if requests.Add(1) > 1 {
			models = append(models, map[string]string{"id": "deepseek-v4-pro"})
		}
		_ = json.NewEncoder(responseWriter).Encode(map[string]any{"data": models})
	}))
	defer server.Close()
	standardProvider := New(atom.ProviderSpec{Name: "deepseek", ModelListURL: server.URL})
	override := atom.ModelInfo{ID: "deepseek-v4-pro", Name: "Configured Pro", Level: 7, Input: []atom.MediaType{atom.Text}, Output: []atom.MediaType{atom.Text}, Tools: false, ContextMax: 8192}
	standardProvider.SetModelConfiguration([]atom.ModelInfo{override})
	standardProvider.SetModels([]atom.ModelInfo{{ID: override.ID, Name: "Stale Pro", Tools: true, ContextMax: 1}})
	models, operationError := standardProvider.Refresh(context.Background())
	testutil.RequireNoError(test, operationError)
	if len(models) != 1 {
		test.Fatal("configuration introduced a model absent from the provider response")
	}
	models, operationError = standardProvider.Refresh(context.Background())
	testutil.RequireNoError(test, operationError)
	for _, model := range models {
		if model.ID == override.ID {
			if !reflect.DeepEqual(model, override) {
				test.Fatalf("explicit configuration was overwritten: %+v", model)
			}
			return
		}
	}
	test.Fatal("the returning model is absent")
}
