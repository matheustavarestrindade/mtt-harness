package embeddinggemma

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestChunkingPreservesEveryUTF8ByteAndPrefixBudget(t *testing.T) {
	text := strings.Repeat("  Matheus\r\n汉字🙂 café e\u0301\t", 3000)
	counter := func(value string) (int, error) { return 11 + utf8.RuneCountInString(value), nil }
	chunks, operationError := splitText(context.Background(), text, 48, counter)
	testutil.RequireNoError(t, operationError)
	if len(chunks) < 2 || strings.Join(chunks, "") != text {
		t.Fatal("chunking lost source bytes")
	}
	for _, chunk := range chunks {
		count, _ := counter(chunk)
		if !utf8.ValidString(chunk) || count > 48 {
			t.Fatalf("invalid chunk: %q", chunk)
		}
	}
}

func TestChunkingStopsOnCancellationAndInvalidInput(t *testing.T) {
	operationContext, cancel := context.WithCancel(context.Background())
	count := 0
	_, operationError := splitText(operationContext, strings.Repeat("x", 10000), 32, func(value string) (int, error) { count++; cancel(); return len(value) + 8, nil })
	if !errors.Is(operationError, context.Canceled) || count != 1 {
		t.Fatalf("cancellation was not propagated: %v, calls %d", operationError, count)
	}
	for _, text := range []string{"bad\x00text", string([]byte{0xff})} {
		if _, operationError := splitText(context.Background(), text, 32, func(string) (int, error) { t.Fatal("invalid text reached tokenizer"); return 0, nil }); operationError == nil {
			t.Fatal("accepted invalid text")
		}
	}
}
