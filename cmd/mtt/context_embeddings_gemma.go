//go:build gemma && cgo

package main

import (
	"context"
	"io"

	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/embedding"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/embedding/embeddinggemma"
)

// This is the only application import of the optional Gemma native adapter.
func newGemmaContextEmbeddings(operationContext context.Context, configuration embedding.ContextConfiguration) (harness.TextEmbedder, string, io.Closer, error) {
	settings := embeddinggemma.Configuration{ModelDirectory: configuration.ModelDirectory, ModelFile: configuration.ModelFile, RuntimeLibrary: configuration.RuntimeLibrary, ChunkTokens: configuration.ChunkTokens, Threads: configuration.Threads}
	identity, operationError := embeddinggemma.Identity(operationContext, settings)
	if operationError != nil {
		return nil, "", nil, operationError
	}
	service := &lazyPluginEmbeddings{create: func() (ownedTextEmbedder, error) { return embeddinggemma.New(settings) }}
	return service, identity, service, nil
}
