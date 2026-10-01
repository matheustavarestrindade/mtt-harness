package embedding

import "context"

// Embedder converts a batch of texts to vectors in one model's space. Results
// preserve input order. Implementations return errors and honor cancellation.
type Embedder interface {
	Embed(operationContext context.Context, texts []string) ([][]float64, error)
}

// Splitter optionally bounds model inputs with the real tokenizer. It preserves
// the complete document across chunks instead of silently truncating it.
type Splitter interface {
	Split(operationContext context.Context, text string) ([]string, error)
}
