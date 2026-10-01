package registry

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/toolsearch"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

type fakeTool struct {
	name        string
	description string
	categories  []string
}

func (tool fakeTool) Name() string             { return tool.name }
func (tool fakeTool) Description() string      { return tool.description }
func (tool fakeTool) Categories() []string     { return tool.categories }
func (tool fakeTool) InputSchema() atom.Schema { return atom.Schema{JSON: []byte(`{"type":"object"}`)} }
func (tool fakeTool) Check(operationContext context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}
func (tool fakeTool) Run(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	return atom.ToolResult{}, nil
}

func names(tools []harness.Tool) []string {
	var result []string
	for _, tool := range tools {
		result = append(result, tool.Name())
	}
	return result
}

func TestFindUsesSemanticScoresAndStrictCategories(test *testing.T) {
	commandTool := fakeTool{name: "bash", description: "Execute commands in a shell", categories: []string{"process"}}
	readTool := fakeTool{name: "read", description: "Read a file", categories: []string{"file"}}
	outputTool := fakeTool{name: "process_output", description: "Read process logs", categories: []string{"process"}}
	// These vectors model the embedding service's response. Retrieval must use
	// that semantic space, rather than count query words or aliases in the text.
	vectors := map[string][]float64{
		toolsearch.Describe(commandTool).Text: {1, 0},
		toolsearch.Describe(readTool).Text:    {0, 1},
		toolsearch.Describe(outputTool).Text:  {0.6, 0.8},
		"shell command exec":                  {1, 0},
		"inspect contents":                    {0, 1},
		"unrelated request":                   {-1, -1},
	}
	index := toolsearch.New(testutil.EmbedderFunc(func(operationContext context.Context, texts []string) ([][]float64, error) {
		result := make([][]float64, len(texts))
		for position, text := range texts {
			vector, present := vectors[text]
			if !present {
				return nil, fmt.Errorf("unexpected embedding input %q", text)
			}
			result[position] = vector
		}
		return result, nil
	}), toolsearch.Options{MinimumSimilarity: 0.5})
	toolRegistry := New(harness.New(), index)
	for _, tool := range []fakeTool{commandTool, readTool, outputTool} {
		testutil.RequireNoError(test, toolRegistry.Add(tool))
	}
	for _, example := range []struct {
		query, category string
		wanted          []string
	}{
		{"shell command exec", "", []string{"bash", "process_output"}},
		{"inspect contents", "", []string{"read", "process_output"}},
		{"inspect contents", " PROCESS ", []string{"process_output"}},
		{"unrelated request", "", nil},
		{"BASH", "", []string{"bash"}},
		{"", "process", []string{"bash", "process_output"}},
		{"", "", nil},
	} {
		found, operationError := toolRegistry.Find(context.Background(), example.query, example.category, 0)
		testutil.RequireNoError(test, operationError)
		if actual := names(found); !reflect.DeepEqual(actual, example.wanted) {
			test.Fatalf("%q with category %q returned %v, want %v", example.query, example.category, actual, example.wanted)
		}
	}
}

func TestFindLimitsAndLiveCatalogChanges(test *testing.T) {
	index := toolsearch.New(testutil.EmbedderFunc(func(operationContext context.Context, texts []string) ([][]float64, error) {
		vectors := make([][]float64, len(texts))
		for position := range vectors {
			vectors[position] = []float64{1, 0}
		}
		return vectors, nil
	}), toolsearch.Options{MinimumSimilarity: 0.5})
	toolRegistry := New(harness.New(), index)
	for position := 0; position < 60; position++ {
		testutil.RequireNoError(test, toolRegistry.Add(fakeTool{name: fmt.Sprintf("tool_%02d", position), categories: []string{"mcp"}}))
	}
	for _, example := range []struct{ limit, count int }{{0, 10}, {-1, 10}, {5, 5}, {100, 50}} {
		found, operationError := toolRegistry.Find(context.Background(), "lookup", "", example.limit)
		testutil.RequireNoError(test, operationError)
		if len(found) != example.count {
			test.Fatalf("limit %d gave %d results", example.limit, len(found))
		}
	}
	first, operationError := toolRegistry.Find(context.Background(), "lookup", "", 5)
	testutil.RequireNoError(test, operationError)
	second, operationError := toolRegistry.Find(context.Background(), "lookup", "", 5)
	testutil.RequireNoError(test, operationError)
	if !reflect.DeepEqual(names(first), names(second)) {
		test.Fatal("identical embeddings gave different order")
	}
	toolRegistry.Remove("tool_00")
	toolRegistry.Upsert(fakeTool{name: "tool_01", description: "changed usage", categories: []string{"different"}})
	found, operationError := toolRegistry.Find(context.Background(), "lookup", "mcp", 1)
	testutil.RequireNoError(test, operationError)
	if !reflect.DeepEqual(names(found), []string{"tool_02"}) {
		test.Fatalf("removed or replaced metadata survived: %v", names(found))
	}
}

func TestFindReportsUnavailableEmbeddingsAndKeepsDirectDiscovery(test *testing.T) {
	toolRegistry := New(harness.New(), nil)
	testutil.RequireNoError(test, toolRegistry.Add(fakeTool{name: "bash", categories: []string{"process"}}))
	if _, operationError := toolRegistry.Find(context.Background(), "shell command exec", "", 0); operationError == nil {
		test.Fatal("unavailable embeddings produced an empty success")
	}
	for _, example := range []struct{ query, category string }{{"bash", ""}, {"", "process"}} {
		found, operationError := toolRegistry.Find(context.Background(), example.query, example.category, 0)
		testutil.RequireNoError(test, operationError)
		if len(found) != 1 || found[0].Name() != "bash" {
			test.Fatal("direct discovery incorrectly required embeddings")
		}
	}
}
