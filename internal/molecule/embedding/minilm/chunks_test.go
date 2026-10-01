//go:build semantic

package minilm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestChunksPreserveUnicodeAndLateSpecifications(test *testing.T) {
	text := strings.Repeat("schema café 😀 field description\n", 100) + "FINAL_PARAMETER: interrupt process group"
	count := func(text string) int { return utf8.RuneCountInString(text) + 2 }
	chunks, operationError := splitText(context.Background(), text, count)
	testutil.RequireNoError(test, operationError)
	if len(chunks) < 2 || strings.Join(chunks, "") != text {
		test.Fatal("chunking omitted document content")
	}
	for _, chunk := range chunks {
		if !utf8.ValidString(chunk) || count(chunk) > maxTokens {
			test.Fatal("invalid or oversized chunk")
		}
	}
	if !strings.Contains(chunks[len(chunks)-1], "FINAL_PARAMETER") {
		test.Fatal("late schema parameter was truncated")
	}
	operationContext, cancel := context.WithCancel(context.Background())
	cancel()
	_, operationError = splitText(operationContext, text, count)
	if !errors.Is(operationError, context.Canceled) {
		test.Fatal("chunking ignored cancellation")
	}
}
