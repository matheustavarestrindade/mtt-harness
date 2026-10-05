package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/toolsearch"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/registry"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func searchRegistry(test *testing.T, index toolsearch.Searcher) *registry.Registry {
	test.Helper()
	toolRegistry := registry.New(harness.New(), index)
	for _, tool := range []harness.Tool{NewBash(nil), NewProcessOutput(nil), NewProcessKill(nil), NewFileActions(false), &Agent{}, Finish{}, NewSearch(toolRegistry)} {
		testutil.RequireNoError(test, toolRegistry.Add(tool))
	}
	return toolRegistry
}

func TestSearchUsesEmbeddingsAndReturnsCompactReferences(test *testing.T) {
	index := toolsearch.New(testutil.EmbedderFunc(func(operationContext context.Context, texts []string) ([][]float64, error) {
		vectors := make([][]float64, len(texts))
		for position, text := range texts {
			vectors[position] = []float64{0, 1}
			if text == "shell command exec" || strings.HasPrefix(text, "Tool: bash\n") {
				vectors[position] = []float64{1, 0}
			}
		}
		return vectors, nil
	}), toolsearch.Options{MinimumSimilarity: 0.5})
	toolRegistry := searchRegistry(test, index)
	result, operationError := NewSearch(toolRegistry).Run(context.Background(), atom.ToolCall{ID: "find-shell", Input: []byte(`{"query":"shell command exec"}`)})
	testutil.RequireNoError(test, operationError)
	var references []atom.ToolReference
	testutil.RequireNoError(test, json.Unmarshal([]byte(result.Text()), &references))
	if len(references) != 1 || references[0].Name != "bash" {
		test.Fatalf("semantic shell query did not return bash: %s", result.Text())
	}
	var entries []map[string]json.RawMessage
	testutil.RequireNoError(test, json.Unmarshal([]byte(result.Text()), &entries))
	for _, entry := range entries {
		if len(entry) != 2 || entry["Name"] == nil || entry["Categories"] == nil {
			test.Fatalf("discovery repeated embedding documents or schemas: %s", result.Text())
		}
	}
}

func TestLexicalDiscoveryWithRealToolDocuments(test *testing.T) {
	toolRegistry := searchRegistry(test, toolsearch.NewLexical(0.01))
	for _, example := range []struct{ query, first string }{
		{"shell command exec", "bash"},
		{"retained stdout stderr logs", "process_output"},
		{"replace literal text matches", "file_actions"},
		{"list directory entries", "file_actions"},
		{"delegate a task to a child agent", "agent"},
	} {
		test.Run(example.query, func(test *testing.T) {
			found, operationError := toolRegistry.Find(context.Background(), example.query, "", 3)
			testutil.RequireNoError(test, operationError)
			if len(found) == 0 || found[0].Name() != example.first {
				var names []string
				for _, tool := range found {
					names = append(names, tool.Name())
				}
				test.Fatalf("lexical query %q did not rank %s first: %v", example.query, example.first, names)
			}
			test.Logf("%q -> %s", example.query, found[0].Name())
		})
	}
}
