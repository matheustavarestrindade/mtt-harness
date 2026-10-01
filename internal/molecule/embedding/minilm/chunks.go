//go:build semantic

package minilm

import (
	"context"
	"fmt"
	"unicode"
	"unicode/utf8"
)

// splitText preserves every byte of a document. The counter uses the model's
// real tokenizer with truncation disabled; a token estimate is not sufficient.
func splitText(operationContext context.Context, text string, count func(string) int) ([]string, error) {
	var chunks []string
	for text != "" {
		if operationError := operationContext.Err(); operationError != nil {
			return nil, operationError
		}
		if count(text) <= maxTokens {
			chunks = append(chunks, text)
			break
		}
		boundaries := []int{0}
		for position := range text {
			if position > 0 {
				boundaries = append(boundaries, position)
			}
		}
		boundaries = append(boundaries, len(text))
		low, high, selected := 1, len(boundaries)-1, 0
		for low <= high {
			if operationError := operationContext.Err(); operationError != nil {
				return nil, operationError
			}
			middle := low + (high-low)/2
			if count(text[:boundaries[middle]]) <= maxTokens {
				selected = boundaries[middle]
				low = middle + 1
				continue
			}
			high = middle - 1
		}
		if selected == 0 {
			return nil, fmt.Errorf("MiniLM cannot fit a document character within its token limit")
		}
		// Prefer a word boundary to avoid changing WordPiece interpretation at
		// the next chunk. A long word still splits safely at a rune boundary.
		for boundary := selected; boundary > selected/2; {
			character, size := utf8.DecodeLastRuneInString(text[:boundary])
			if unicode.IsSpace(character) {
				selected = boundary
				break
			}
			boundary -= size
		}
		for count(text[:selected]) > maxTokens {
			_, size := utf8.DecodeLastRuneInString(text[:selected])
			selected -= size
			if selected == 0 {
				return nil, fmt.Errorf("MiniLM document chunk exceeds its token limit")
			}
		}
		chunks = append(chunks, text[:selected])
		text = text[selected:]
	}
	if len(chunks) == 0 {
		return []string{""}, nil
	}
	return chunks, nil
}
