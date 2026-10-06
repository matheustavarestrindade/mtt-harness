//go:build semantic

package tools

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/embedding/minilm"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/toolsearch"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

// The image supplies pinned model assets; ordinary local tests may opt in with
// MTT_TEST_SEARCH_MODEL_DIRECTORY. No server or network inference is involved.
func TestSemanticDiscoveryWithRealEmbeddings(test *testing.T) {
	modelDirectory := os.Getenv("MTT_TEST_SEARCH_MODEL_DIRECTORY")
	if modelDirectory == "" {
		test.Skip("MTT_TEST_SEARCH_MODEL_DIRECTORY is not set")
	}
	encoder, operationError := minilm.New(modelDirectory)
	testutil.RequireNoError(test, operationError)
	index := toolsearch.New(encoder, toolsearch.Options{MinimumSimilarity: 0.3})
	defer func() { testutil.RequireNoError(test, index.Close()) }()
	toolRegistry := searchRegistry(test, index)
	for _, example := range []struct{ query, first string }{
		{"shell command exec", "bash"},
		{"execute terminal commands", "bash"},
		{"inspect output from a running server", "process_output"},
		{"replace existing text in a source file", "file_actions"},
		{"find files recursively by a glob pattern", "file_actions"},
		{"delegate a smaller task to a child", "agent"},
	} {
		test.Run(example.query, func(test *testing.T) {
			started := time.Now()
			found, operationError := toolRegistry.Find(context.Background(), example.query, "", 3)
			testutil.RequireNoError(test, operationError)
			if len(found) == 0 || found[0].Name() != example.first {
				test.Fatalf("MiniLM query %q did not rank %s first: %v", example.query, example.first, found)
			}
			test.Logf("%q -> %s in %s", example.query, found[0].Name(), time.Since(started))
		})
	}
	for _, query := range []string{"shell command exec", "execute terminal commands", "inspect output from a running server"} {
		started := time.Now()
		_, operationError := toolRegistry.Find(context.Background(), query, "", 3)
		testutil.RequireNoError(test, operationError)
		test.Logf("warm %q: %s", query, time.Since(started))
	}
}
