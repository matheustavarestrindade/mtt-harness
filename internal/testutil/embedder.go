package testutil

import "context"

type EmbedderFunc func(operationContext context.Context, texts []string) ([][]float64, error)

func (embedder EmbedderFunc) Embed(operationContext context.Context, texts []string) ([][]float64, error) {
	return embedder(operationContext, texts)
}
