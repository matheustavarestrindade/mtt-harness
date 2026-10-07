//go:build !semantic

package main

import (
	"context"
	"fmt"
	"io"

	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/toolsearch"
)

func newSemanticSearch(operationContext context.Context, configuration toolsearch.Configuration) (toolsearch.Searcher, error) {
	return nil, fmt.Errorf("semantic search was not compiled; build with -tags semantic or choose lexical mode")
}

func newMiniLMContextEmbeddings(modelDirectory string) (harness.TextEmbedder, string, io.Closer, error) {
	return nil, "", nil, fmt.Errorf("context embeddings require the semantic build")
}
