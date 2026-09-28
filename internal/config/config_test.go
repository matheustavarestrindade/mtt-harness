package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestLoadOrDefault(test *testing.T) {
	bootstrap, operationError := LoadOrDefault(filepath.Join(test.TempDir(), "missing.json"))
	testutil.RequireNoError(test, operationError)

	if bootstrap.Port != 8080 || bootstrap.ProvidersFile != "providers.json" {
		test.Fatalf("default = %+v", bootstrap)
	}
	path := filepath.Join(test.TempDir(), "mtt.json")
	testutil.RequireNoError(test, os.WriteFile(path, []byte(`{"port": 9999, "database_url": "postgres://x", "test_provider": true}`), 0o644))

	bootstrap, operationError = LoadOrDefault(path)
	testutil.RequireNoError(test, operationError)

	if bootstrap.Port != 9999 || bootstrap.DatabaseURL != "postgres://x" || !bootstrap.TestProvider {
		test.Fatalf("loaded = %+v", bootstrap)
	}
}
