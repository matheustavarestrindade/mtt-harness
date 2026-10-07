//go:build semantic

package main

import (
	"context"
	"errors"
	"io"

	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/embedding/minilm"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/toolsearch"
)

// This file is the only application dependency on the optional model adapter.
// Removing it and the adapter leaves the default lexical-only build usable.
func newSemanticSearch(operationContext context.Context, configuration toolsearch.Configuration) (toolsearch.Searcher, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	encoder, operationError := minilm.New(configuration.ModelDirectory)
	if operationError != nil {
		return nil, operationError
	}
	if operationError := operationContext.Err(); operationError != nil {
		return nil, errors.Join(operationError, encoder.Close())
	}
	return toolsearch.New(encoder, toolsearch.Options{MinimumSimilarity: configuration.SemanticMinimumSimilarity}), nil
}

func newContextEmbeddings(modelDirectory string) (harness.TextEmbedder, string, io.Closer, error) {
	identity, operationError := embeddingAssetIdentity(modelDirectory)
	if operationError != nil {
		return nil, "", nil, operationError
	}
	service := &lazyPluginEmbeddings{create: func() (ownedTextEmbedder, error) { return minilm.New(modelDirectory) }}
	return service, identity, service, nil
}
