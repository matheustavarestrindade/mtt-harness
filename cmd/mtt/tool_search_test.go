package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/toolsearch"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestToolSearchLexicalConfigurationNeedsNoModel(test *testing.T) {
	path := filepath.Join(test.TempDir(), "providers.json")
	testutil.RequireNoError(test, os.WriteFile(path, []byte(`{"tool_search":{"mode":"lexical","model_directory":"","lexical_minimum_similarity":0.01}}`), 0600))
	searcher := loadToolSearch(context.Background(), path)
	defer func() { testutil.RequireNoError(test, searcher.Close()) }()
	matches, operationError := searcher.Search(context.Background(), []toolsearch.Document{{ID: "bash", Text: "execute shell commands"}}, "shell command")
	testutil.RequireNoError(test, operationError)
	if len(matches) != 1 || matches[0].ID != "bash" || searcher.SelectedMode() != toolsearch.ModeLexical {
		test.Fatalf("model-free configuration was not usable: %v", matches)
	}
}
