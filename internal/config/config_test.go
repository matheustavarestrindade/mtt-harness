package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrDefault(t *testing.T) {
	bootstrap, err := LoadOrDefault(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap.Port != 8080 || bootstrap.ProvidersFile != "providers.json" {
		t.Fatalf("default = %+v", bootstrap)
	}
	path := filepath.Join(t.TempDir(), "mtt.json")
	if err := os.WriteFile(path, []byte(`{"port": 9999, "database_url": "postgres://x", "test_provider": true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	bootstrap, err = LoadOrDefault(path)
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap.Port != 9999 || bootstrap.DatabaseURL != "postgres://x" || !bootstrap.TestProvider {
		t.Fatalf("loaded = %+v", bootstrap)
	}
}
