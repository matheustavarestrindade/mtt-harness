package toolsearch

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestLexicalRanksPartialQueriesAndUpdatesCorpus(test *testing.T) {
	index := NewLexical(0.01)
	documents := []Document{{ID: "bash", Text: "execute shell command"}, {ID: "read", Text: "read file contents"}, {ID: "logs", Text: "read process output"}}
	matches, operationError := index.Search(context.Background(), documents, "shell command exec")
	testutil.RequireNoError(test, operationError)
	if len(matches) != 1 || matches[0].ID != "bash" {
		test.Fatalf("partial lexical query failed: %v", matches)
	}
	first := append([]Match(nil), matches...)
	matches, operationError = index.Search(context.Background(), documents, "SHELL COMMAND EXEC")
	testutil.RequireNoError(test, operationError)
	if !reflect.DeepEqual(first, matches) {
		test.Fatal("lexical scores changed for the same query")
	}
	documents[0].Text = "database transaction"
	matches, operationError = index.Search(context.Background(), documents, "shell command")
	testutil.RequireNoError(test, operationError)
	if len(matches) != 0 {
		test.Fatalf("changed document retained vocabulary: %v", matches)
	}
	documents = documents[1:]
	matches, operationError = index.Search(context.Background(), documents, "database")
	testutil.RequireNoError(test, operationError)
	if len(matches) != 0 {
		test.Fatalf("removed document retained vocabulary: %v", matches)
	}
}

func TestLexicalSingleDocumentUnknownTermsAndThreshold(test *testing.T) {
	documents := []Document{{ID: "read", Text: "read unicode café file"}}
	for _, example := range []struct {
		query   string
		minimum float64
		count   int
	}{
		{"CAFÉ", 0.01, 1}, {"café", 1, 0}, {"absent", 0, 0}, {"", 0, 0},
	} {
		matches, operationError := NewLexical(example.minimum).Search(context.Background(), documents, example.query)
		testutil.RequireNoError(test, operationError)
		if len(matches) != example.count {
			test.Fatalf("%q returned %v", example.query, matches)
		}
	}
	operationContext, cancel := context.WithCancel(context.Background())
	cancel()
	_, operationError := NewLexical(0).Search(operationContext, documents, "read")
	if !errors.Is(operationError, context.Canceled) {
		test.Fatalf("lexical cancellation was lost: %v", operationError)
	}
}

func BenchmarkLexicalWarmSearch(benchmark *testing.B) {
	index := NewLexical(0.01)
	documents := []Document{{ID: "bash", Text: "execute shell commands"}, {ID: "read", Text: "read file contents"}, {ID: "logs", Text: "read process output"}}
	_, operationError := index.Search(context.Background(), documents, "shell command exec")
	if operationError != nil {
		benchmark.Fatal(operationError)
	}
	benchmark.ResetTimer()
	for benchmark.Loop() {
		_, operationError := index.Search(context.Background(), documents, "shell command exec")
		if operationError != nil {
			benchmark.Fatal(operationError)
		}
	}
}
