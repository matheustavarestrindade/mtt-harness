//go:build semantic

package minilm

import (
	"context"
	"errors"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestEncoderRealTokenizerPreservesFullDocumentAndLifecycle(test *testing.T) {
	modelDirectory := os.Getenv("MTT_TEST_SEARCH_MODEL_DIRECTORY")
	if modelDirectory == "" {
		test.Skip("MTT_TEST_SEARCH_MODEL_DIRECTORY is not set")
	}
	encoder, operationError := New(modelDirectory)
	testutil.RequireNoError(test, operationError)
	defer func() { testutil.RequireNoError(test, encoder.Close()) }()
	text := strings.Repeat("Input schema with café file parameters and JSON fields.\n", 80) + "FINAL_PARAMETER: terminate shell descendants"
	chunks, operationError := encoder.Split(context.Background(), text)
	testutil.RequireNoError(test, operationError)
	if len(chunks) < 2 || strings.Join(chunks, "") != text {
		test.Fatal("real tokenizer truncated the document")
	}
	for _, chunk := range chunks {
		if encoder.tokenCount(chunk) > maxTokens {
			test.Fatal("chunk exceeds actual WordPiece limit")
		}
	}
	vectors, operationError := encoder.Embed(context.Background(), []string{"execute terminal commands"})
	testutil.RequireNoError(test, operationError)
	if len(vectors) != 1 || len(vectors[0]) != 384 {
		test.Fatal("wrong embedding dimensions")
	}
	magnitude := 0.0
	for _, value := range vectors[0] {
		magnitude += value * value
	}
	if math.Abs(magnitude-1) > 0.0001 {
		test.Fatalf("mean-pooled model output was not normalized: %f", magnitude)
	}
	_, operationError = encoder.Embed(context.Background(), []string{text})
	if operationError == nil {
		test.Fatal("oversized input was silently truncated")
	}
	operationContext, cancel := context.WithCancel(context.Background())
	cancel()
	_, operationError = encoder.Embed(operationContext, []string{"ignored"})
	if !errors.Is(operationError, context.Canceled) {
		test.Fatal("encoder ignored cancellation")
	}
	testutil.RequireNoError(test, encoder.Close())
	_, operationError = encoder.Embed(context.Background(), []string{"late"})
	if !errors.Is(operationError, ErrClosed) {
		test.Fatal("closed encoder accepted work")
	}
}
