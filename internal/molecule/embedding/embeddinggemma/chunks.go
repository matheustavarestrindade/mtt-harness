package embeddinggemma

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

// splitText preserves source bytes; the counter includes task prefixes and
// special tokens. Bounded windows avoid tokenizing an entire large archive.
func splitText(operationContext context.Context, text string, limit int, countTokens func(string) (int, error)) ([]string, error) {
	if limit < 1 {
		return nil, fmt.Errorf("EmbeddingGemma chunk token limit is not positive")
	}
	if !utf8.ValidString(text) {
		return nil, fmt.Errorf("EmbeddingGemma input is not valid UTF-8")
	}
	if strings.IndexByte(text, 0) >= 0 {
		return nil, fmt.Errorf("EmbeddingGemma input contains a NUL byte")
	}
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	if text == "" {
		return []string{""}, nil
	}
	var chunks []string
	for len(text) > 0 {
		if operationError := operationContext.Err(); operationError != nil {
			return nil, operationError
		}
		end := min(len(text), limit*16)
		for end < len(text) && !utf8.RuneStart(text[end]) {
			end--
		}
		tokens, operationError := countTokens(text[:end])
		if operationError != nil {
			return nil, operationError
		}
		if tokens > limit {
			low, high, best := 1, end, 0
			for low <= high {
				if operationError := operationContext.Err(); operationError != nil {
					return nil, operationError
				}
				candidate := low + (high-low)/2
				for candidate > 0 && candidate < len(text) && !utf8.RuneStart(text[candidate]) {
					candidate--
				}
				if candidate == 0 {
					_, width := utf8.DecodeRuneInString(text)
					candidate = width
				}
				tokens, operationError := countTokens(text[:candidate])
				if operationError != nil {
					return nil, operationError
				}
				if tokens <= limit {
					best = candidate
					low = candidate + 1
					for low < len(text) && !utf8.RuneStart(text[low]) {
						low++
					}
					continue
				}
				high = candidate - 1
			}
			if best == 0 {
				return nil, fmt.Errorf("EmbeddingGemma token limit cannot contain one input character")
			}
			end = best
		}
		chunks = append(chunks, text[:end])
		text = text[end:]
	}
	return chunks, nil
}
