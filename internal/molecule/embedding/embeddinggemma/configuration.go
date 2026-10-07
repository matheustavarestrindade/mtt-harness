// Package embeddinggemma contains the removable text-only EmbeddingGemma 2 adapter.
// Native inference is included only with the gemma build tag and CGO.
package embeddinggemma

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	maxTokens         = 8192
	dimensions        = 768
	featureDimensions = 512
	documentPrefix    = "title: none | text: "
	queryPrefix       = "task: search result | query: "
)

// Configuration contains local assets and bounded CPU execution settings.
type Configuration struct {
	ModelDirectory string
	ModelFile      string
	RuntimeLibrary string
	ChunkTokens    int
	Threads        int
}

func (configuration Configuration) validate() error {
	if configuration.ModelDirectory == "" || configuration.RuntimeLibrary == "" {
		return fmt.Errorf("EmbeddingGemma model directory or runtime library is empty")
	}
	if configuration.ModelFile == "" || !filepath.IsLocal(configuration.ModelFile) {
		return fmt.Errorf("EmbeddingGemma model file is not a local relative path")
	}
	if configuration.ChunkTokens < 32 || configuration.ChunkTokens > maxTokens {
		return fmt.Errorf("EmbeddingGemma chunk token limit must be between 32 and %d", maxTokens)
	}
	if configuration.Threads < 1 || configuration.Threads > 64 {
		return fmt.Errorf("EmbeddingGemma thread count must be between 1 and 64")
	}
	return nil
}

// Identity binds vectors to the exact text model, tokenizer and preprocessing.
// It performs no inference and supports lazy resource initialization at startup.
func Identity(operationContext context.Context, configuration Configuration) (string, error) {
	if operationError := configuration.validate(); operationError != nil {
		return "", operationError
	}
	digest := sha256.New()
	_, _ = fmt.Fprintf(digest, "embeddinggemma2:text:v1:literal-special-tokens:chunk=%d:dimensions=%d:%s:%s\x00", configuration.ChunkTokens, dimensions, documentPrefix, queryPrefix)
	for _, name := range []string{configuration.ModelFile, configuration.ModelFile + "_data", "tokenizer.json", "tokenizer_config.json", "config.json"} {
		if operationError := operationContext.Err(); operationError != nil {
			return "", operationError
		}
		file, operationError := os.Open(filepath.Join(configuration.ModelDirectory, name))
		if operationError != nil {
			return "", operationError
		}
		_, _ = io.WriteString(digest, name+"\x00")
		_, copyError := io.Copy(digest, file)
		closeError := file.Close()
		if copyError != nil {
			return "", copyError
		}
		if closeError != nil {
			return "", closeError
		}
	}
	return "embeddinggemma2:sha256:" + hex.EncodeToString(digest.Sum(nil)), operationContext.Err()
}
