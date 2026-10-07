package main

import (
	"context"
	"fmt"
	"io"

	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/embedding"
)

func newContextEmbeddings(operationContext context.Context, configuration embedding.ContextConfiguration) (harness.TextEmbedder, string, io.Closer, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return nil, "", nil, operationError
	}
	switch configuration.Backend {
	case "disabled":
		return nil, "", nil, nil
	case "minilm":
		return newMiniLMContextEmbeddings(configuration.ModelDirectory)
	case "embeddinggemma2":
		return newGemmaContextEmbeddings(operationContext, configuration)
	default:
		return nil, "", nil, fmt.Errorf("context embedding backend %q is not supported", configuration.Backend)
	}
}
