package provider

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.json")
	data := `{
		"providers": [
			{
				"name": "example",
				"api_url": "https://api.example.com/v1",
				"model_list_url": "https://api.example.com/v1/models",
				"api_key_env": "MTT_TEST_PROVIDER_KEY",
				"refresh_hours": 12,
				"prices": {"m1": {"currency": "USD", "input": 2, "output": 4}},
				"models": [{"id": "m1", "tools": true, "context_max": 1000, "input": ["text", "image"], "output": ["text"]}]
			}
		]
	}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MTT_TEST_PROVIDER_KEY", "secret-value")
	configs, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 1 {
		t.Fatalf("configs = %d", len(configs))
	}
	config := configs[0]
	if config.Spec.Name != "example" {
		t.Fatalf("spec = %+v", config.Spec)
	}
	if config.Spec.Interval.Hours() != 12 {
		t.Fatalf("interval = %v", config.Spec.Interval)
	}
	if len(config.Models) != 1 || config.Models[0].Prices == nil || config.Models[0].Prices.Input != 2 {
		t.Fatalf("models = %+v", config.Models)
	}
	if len(config.Models[0].Input) != 2 || config.Models[0].Input[1] != "image" {
		t.Fatalf("input media = %+v", config.Models[0].Input)
	}
}
