package toolsearch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestConfigurationDefaultsAndRelativeAssets(test *testing.T) {
	path := filepath.Join(test.TempDir(), "providers.json")
	configuration, operationError := LoadConfiguration(path)
	testutil.RequireNoError(test, operationError)
	if configuration != DefaultConfiguration() {
		test.Fatal("missing file changed defaults")
	}
	testutil.RequireNoError(test, os.WriteFile(path, []byte(`{"providers":[],"tool_search":{"mode":"semantic","model_directory":"models/minilm","semantic_minimum_similarity":0}}`), 0600))
	configuration, operationError = LoadConfiguration(path)
	testutil.RequireNoError(test, operationError)
	if configuration.ModelDirectory != filepath.Join(filepath.Dir(path), "models/minilm") || configuration.SemanticMinimumSimilarity != 0 || configuration.LexicalMinimumSimilarity != 0.01 {
		test.Fatalf("configuration lost a path or explicit zero: %+v", configuration)
	}
}

func TestConfigurationRejectsInvalidModesAndBounds(test *testing.T) {
	for _, source := range []string{`{"mode":"unknown"}`, `{"mode":"semantic","model_directory":""}`, `{"semantic_minimum_similarity":1.1}`, `{"lexical_minimum_similarity":-0.1}`, `{"mode":3}`} {
		path := filepath.Join(test.TempDir(), "providers.json")
		testutil.RequireNoError(test, os.WriteFile(path, []byte(`{"tool_search":`+source+`}`), 0600))
		if _, operationError := LoadConfiguration(path); operationError == nil {
			test.Fatalf("accepted invalid configuration %s", source)
		}
	}
}
