package toolsearch

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestIndexCachesDocumentsAndRefreshesChangedSpecs(test *testing.T) {
	var inputs [][]string
	vectors := map[string][]float64{
		"document: first specification":   {1, 0},
		"document: second specification":  {0.6, 0.8},
		"document: changed specification": {0, 1},
		"query: run something":            {1, 0},
	}
	index := New(testutil.EmbedderFunc(func(operationContext context.Context, texts []string) ([][]float64, error) {
		inputs = append(inputs, append([]string(nil), texts...))
		result := make([][]float64, len(texts))
		for position, text := range texts {
			result[position] = vectors[text]
		}
		return result, nil
	}), Options{DocumentPrefix: "document: ", QueryPrefix: "query: ", MinimumSimilarity: 0.5})
	documents := []Document{{ID: "first", Text: "first specification"}, {ID: "second", Text: "second specification"}}
	matches, operationError := index.Search(context.Background(), documents, "run something")
	testutil.RequireNoError(test, operationError)
	if len(matches) != 2 || matches[0].ID != "first" || matches[1].ID != "second" {
		test.Fatalf("wrong cosine ranking: %v", matches)
	}
	_, operationError = index.Search(context.Background(), documents, "run something")
	testutil.RequireNoError(test, operationError)
	if !reflect.DeepEqual(inputs[1], []string{"query: run something"}) {
		test.Fatalf("unchanged documents were embedded again: %v", inputs[1])
	}
	documents[0].Text = "changed specification"
	matches, operationError = index.Search(context.Background(), documents, "run something")
	testutil.RequireNoError(test, operationError)
	if !reflect.DeepEqual(inputs[2], []string{"document: changed specification", "query: run something"}) {
		test.Fatalf("changed spec did not invalidate its vector: %v", inputs[2])
	}
	if len(matches) != 1 || matches[0].ID != "second" {
		test.Fatalf("stale vector remained searchable: %v", matches)
	}
	_, operationError = index.Search(context.Background(), documents[1:], "run something")
	testutil.RequireNoError(test, operationError)
	if _, present := index.cache["first"]; present {
		test.Fatal("removed document remains cached")
	}
}

type chunkEmbedder struct{ testutil.EmbedderFunc }

func (embedder chunkEmbedder) Split(operationContext context.Context, text string) ([]string, error) {
	return strings.Split(text, "|"), operationContext.Err()
}

func TestIndexFindsLateDocumentChunksAndCachesThem(test *testing.T) {
	var inputs [][]string
	embedder := chunkEmbedder{testutil.EmbedderFunc(func(operationContext context.Context, texts []string) ([][]float64, error) {
		inputs = append(inputs, append([]string(nil), texts...))
		vectors := make([][]float64, len(texts))
		for position, text := range texts {
			vectors[position] = []float64{0, 1}
			if text == "tail parameter" || text == "query" {
				vectors[position] = []float64{1, 0}
			}
		}
		return vectors, nil
	})}
	index := New(embedder, Options{MinimumSimilarity: 0.5})
	documents := []Document{{ID: "tool", Text: "first schema|tail parameter"}}
	matches, operationError := index.Search(context.Background(), documents, "query")
	testutil.RequireNoError(test, operationError)
	if len(matches) != 1 || matches[0].ID != "tool" || matches[0].Similarity != 1 {
		test.Fatal("late parameter did not rank by its own vector")
	}
	_, operationError = index.Search(context.Background(), documents, "query")
	testutil.RequireNoError(test, operationError)
	if !reflect.DeepEqual(inputs[1], []string{"query"}) {
		test.Fatal("document chunks were not cached")
	}
}

func TestIndexRejectsBrokenVectorsAndRecovers(test *testing.T) {
	for _, vectors := range [][][]float64{{{0, 0}, {1, 0}}, {{1, 0, 0}, {1, 0}}, {{1, 0}}} {
		broken := true
		index := New(testutil.EmbedderFunc(func(operationContext context.Context, texts []string) ([][]float64, error) {
			if broken {
				return vectors, nil
			}
			return [][]float64{{1, 0}, {1, 0}}, nil
		}), Options{MinimumSimilarity: 0.5})
		documents := []Document{{ID: "tool", Text: "spec"}}
		if _, operationError := index.Search(context.Background(), documents, "query"); operationError == nil {
			test.Fatal("broken embedding was accepted")
		}
		broken = false
		matches, operationError := index.Search(context.Background(), documents, "query")
		testutil.RequireNoError(test, operationError)
		if len(matches) != 1 {
			test.Fatal("index did not recover after a bad response")
		}
	}
}

func TestIndexCancellationDoesNotBlockOtherCallers(test *testing.T) {
	entered := make(chan struct{}, 1)
	var calls atomic.Int32
	index := New(testutil.EmbedderFunc(func(operationContext context.Context, texts []string) ([][]float64, error) {
		calls.Add(1)
		entered <- struct{}{}
		<-operationContext.Done()
		return nil, operationContext.Err()
	}), Options{})
	documents := []Document{{ID: "tool", Text: "spec"}}
	operationContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	completed := make(chan error, 1)
	go func() {
		_, operationError := index.Search(operationContext, documents, "query")
		completed <- operationError
	}()
	<-entered
	waitContext, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	if _, operationError := index.Search(waitContext, documents, "query"); !errors.Is(operationError, context.DeadlineExceeded) {
		test.Fatalf("waiting caller did not cancel: %v", operationError)
	}
	if calls.Load() != 1 {
		test.Fatal("abandoned caller started an embedding request")
	}
	cancel()
	if operationError := <-completed; !errors.Is(operationError, context.Canceled) {
		test.Fatalf("embedding cancellation was lost: %v", operationError)
	}
}
