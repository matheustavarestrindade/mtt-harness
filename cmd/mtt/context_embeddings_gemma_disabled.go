//go:build !gemma || !cgo

package main

import (
	"context"
	"fmt"
	"io"

	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/embedding"
)

func newGemmaContextEmbeddings(operationContext context.Context, configuration embedding.ContextConfiguration) (harness.TextEmbedder, string, io.Closer, error) {
	return nil, "", nil, fmt.Errorf("EmbeddingGemma 2 support is unavailable in this build")
}
