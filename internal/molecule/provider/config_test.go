package provider

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestLoadFile(test *testing.T) {
	path := filepath.Join(test.TempDir(), "providers.json")
	data := `{
		"providers": [
			{
				"name": "example",
				"api_url": "https://api.example.com/v1",
				"model_list_url": "https://api.example.com/v1/models",
				"api_key_env": "MTT_TEST_PROVIDER_KEY",
				"refresh_hours": 12,
				"prices": {"m1": {"currency": "USD", "input": 2, "output": 4}},
				"models": [{"id": "m1", "name": "Example Model", "tools": true, "context_max": 1000, "input": ["text", "image"], "output": ["text"]}]
			}
		]
	}`
	testutil.RequireNoError(test, os.WriteFile(path, []byte(data), 0o644))

	test.Setenv("MTT_TEST_PROVIDER_KEY", "secret-value")
	configs, operationError := LoadFile(path)
	testutil.RequireNoError(test, operationError)

	if len(configs) != 1 {
		test.Fatalf("configs = %d", len(configs))
	}
	configuration := configs[0]
	if configuration.Spec.Name != "example" {
		test.Fatalf("spec = %+v", configuration.Spec)
	}
	if configuration.Spec.Interval.Hours() != 12 {
		test.Fatalf("interval = %v", configuration.Spec.Interval)
	}
	if len(configuration.Models) != 1 || configuration.Prices["m1"].Input != 2 {
		test.Fatalf("models = %+v", configuration.Models)
	}
	if len(configuration.Models[0].Input) != 2 || configuration.Models[0].Input[1] != "image" {
		test.Fatalf("input media = %+v", configuration.Models[0].Input)
	}
	if configuration.Models[0].Name == nil || *configuration.Models[0].Name != "Example Model" {
		test.Fatalf("configured model display name = %v", configuration.Models[0].Name)
	}
}
