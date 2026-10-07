//go:build gemma && cgo

package embeddinggemma

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func testConfiguration(test *testing.T) Configuration {
	test.Helper()
	directory, library := os.Getenv("MTT_TEST_CONTEXT_MODEL_DIRECTORY"), os.Getenv("MTT_TEST_CONTEXT_RUNTIME_LIBRARY")
	if directory == "" || library == "" {
		test.Skip("native Gemma test assets are not configured")
	}
	return Configuration{ModelDirectory: directory, ModelFile: "onnx/model_quantized.onnx", RuntimeLibrary: library, ChunkTokens: 1024, Threads: 2}
}

func testEncoder(test *testing.T) *Encoder {
	test.Helper()
	encoder, operationError := New(testConfiguration(test))
	testutil.RequireNoError(test, operationError)
	test.Cleanup(func() { testutil.RequireNoError(test, encoder.Close()) })
	return encoder
}

func TestNativeVectorsMatchIndependentReference(t *testing.T) {
	encoder := testEncoder(t)
	data, operationError := os.ReadFile(filepath.Join("testdata", "reference-q8.json"))
	testutil.RequireNoError(t, operationError)
	var reference []struct {
		Text      string
		Tokens    []uint32
		Embedding []float64
	}
	testutil.RequireNoError(t, json.Unmarshal(data, &reference))
	for _, entry := range reference {
		tokens, operationError := encoder.tokenize(entry.Text)
		testutil.RequireNoError(t, operationError)
		if !slices.Equal(tokens, entry.Tokens) {
			t.Fatalf("tokens differ from reference for %q", entry.Text)
		}
		var vectors [][]float64
		if strings.HasPrefix(entry.Text, queryPrefix) {
			vectors, operationError = encoder.EmbedQueries(context.Background(), []string{strings.TrimPrefix(entry.Text, queryPrefix)})
		} else {
			vectors, operationError = encoder.Embed(context.Background(), []string{strings.TrimPrefix(entry.Text, documentPrefix)})
		}
		testutil.RequireNoError(t, operationError)
		if len(vectors) != 1 || len(vectors[0]) != 768 {
			t.Fatal("wrong embedding shape")
		}
		var norm float64
		for position, value := range vectors[0] {
			norm += value * value
			if math.Abs(value-entry.Embedding[position]) > 2e-5 {
				t.Fatalf("vector differs from reference for %q at dimension %d", entry.Text, position)
			}
		}
		if math.Abs(norm-1) > 1e-8 {
			t.Fatalf("vector is not normalized: %g", norm)
		}
	}
}

func TestNativeTokenizerChunksWithoutTruncation(t *testing.T) {
	encoder := testEncoder(t)
	text := strings.Repeat("中文🙂 café\r\n some code: <|image|>  ", 1200)
	chunks, operationError := encoder.Split(context.Background(), text)
	testutil.RequireNoError(t, operationError)
	if strings.Join(chunks, "") != text || len(chunks) < 2 {
		t.Fatal("tokenizer chunking changed source text")
	}
	for _, chunk := range chunks {
		for _, prefix := range []string{documentPrefix, queryPrefix} {
			tokens, operationError := encoder.tokenize(prefix + chunk)
			testutil.RequireNoError(t, operationError)
			if len(tokens) > encoder.configuration.ChunkTokens {
				t.Fatalf("chunk exceeded budget: %d", len(tokens))
			}
		}
	}
	_, operationError = encoder.Embed(context.Background(), []string{"The source code contains <|image|> and <|audio|> as literal text."})
	testutil.RequireNoError(t, operationError)
	if _, operationError := encoder.Embed(context.Background(), []string{strings.Repeat("word ", maxTokens)}); operationError == nil {
		t.Fatal("oversized input was silently truncated")
	}
}

func TestNativeCancellationDoesNotPoisonNextRequest(t *testing.T) {
	encoder := testEncoder(t)
	tokens, operationError := encoder.tokenize(documentPrefix + strings.Repeat("word ", 2048))
	testutil.RequireNoError(t, operationError)
	operationContext, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, operationError = encoder.runInference(operationContext, tokens)
	if !errors.Is(operationError, context.DeadlineExceeded) {
		t.Fatalf("expected cancellation, got %v", operationError)
	}
	if time.Since(started) > 5*time.Second {
		t.Fatal("native cancellation did not stop inference promptly")
	}
	_, operationError = encoder.EmbedQueries(context.Background(), []string{"database"})
	testutil.RequireNoError(t, operationError)
}

func TestNativeEnvironmentOutlivesIndividualEncoders(t *testing.T) {
	first := testEncoder(t)
	second := testEncoder(t)
	testutil.RequireNoError(t, first.Close())
	if _, operationError := first.Embed(context.Background(), []string{"closed"}); !errors.Is(operationError, ErrClosed) {
		t.Fatalf("closed encoder returned %v", operationError)
	}
	_, operationError := second.Embed(context.Background(), []string{"still available"})
	testutil.RequireNoError(t, operationError)
	testutil.RequireNoError(t, second.Close())
	third := testEncoder(t)
	_, operationError = third.EmbedQueries(context.Background(), []string{"reopened"})
	testutil.RequireNoError(t, operationError)
}
