//go:build semantic

// Package minilm is the optional, removable Hugot adapter. Only the semantic
// build factory imports it. The core search contract and lexical backend do not.
package minilm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gomlx/go-huggingface/tokenizers/api"
	"github.com/knights-analytics/hugot"
	"github.com/knights-analytics/hugot/pipelines"
)

const maxTokens = 256

var ErrClosed = errors.New("MiniLM encoder is closed")

// Encoder owns one pure-Go inference session. The gate serializes Hugot's
// tokenizer options, tensors, and teardown. No native library or server is used.
type Encoder struct {
	session    *hugot.Session
	pipeline   *pipelines.FeatureExtractionPipeline
	gate       chan struct{}
	closed     bool
	closeError error
}

func New(modelDirectory string) (*Encoder, error) {
	for _, name := range []string{"model.onnx", "tokenizer.json", "config.json", "sentence_bert_config.json"} {
		information, operationError := os.Stat(filepath.Join(modelDirectory, name))
		if operationError != nil {
			return nil, fmt.Errorf("MiniLM model asset %s: %w", name, operationError)
		}
		if !information.Mode().IsRegular() {
			return nil, fmt.Errorf("MiniLM model asset %s is not a regular file", name)
		}
	}
	session, operationError := hugot.NewGoSession()
	if operationError != nil {
		return nil, operationError
	}
	pipeline, operationError := hugot.NewPipeline(session, hugot.FeatureExtractionConfig{
		ModelPath: modelDirectory, Name: "mtt-tool-search", OnnxFilename: "model.onnx",
		Options: []hugot.FeatureExtractionOption{pipelines.WithNormalization()},
	})
	if operationError != nil {
		return nil, errors.Join(operationError, session.Destroy())
	}
	// Disable library truncation. We split by actual WordPiece token counts and
	// verify every chunk before inference, retaining the complete specification.
	operationError = pipeline.GetModel().Tokenizer.GoTokenizer.Tokenizer.With(api.EncodeOptions{
		AddSpecialTokens: true, IncludeSpans: true, IncludeSpecialTokensMask: true, MaxLen: 0,
	})
	if operationError != nil {
		return nil, errors.Join(operationError, session.Destroy())
	}
	return &Encoder{session: session, pipeline: pipeline, gate: make(chan struct{}, 1)}, nil
}

// Embed checks cancellation between bounded inference chunks. Hugot v0.7's Go
// execution cannot interrupt the current chunk; its result is discarded on
// cancellation, and no following chunk starts. Close joins that computation.
func (encoder *Encoder) Embed(operationContext context.Context, texts []string) ([][]float64, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	select {
	case encoder.gate <- struct{}{}:
	case <-operationContext.Done():
		return nil, operationContext.Err()
	}
	defer func() { <-encoder.gate }()
	if encoder.closed {
		return nil, ErrClosed
	}
	vectors := make([][]float64, len(texts))
	for position, text := range texts {
		if operationError := operationContext.Err(); operationError != nil {
			return nil, operationError
		}
		if encoder.countWordPieceTokens(text) > maxTokens {
			return nil, fmt.Errorf("MiniLM input exceeds %d tokens; split the document before encoding", maxTokens)
		}
		result, operationError := encoder.pipeline.RunPipeline([]string{text})
		if operationError != nil {
			return nil, operationError
		}
		if operationError := operationContext.Err(); operationError != nil {
			return nil, operationError
		}
		if len(result.Embeddings) != 1 || len(result.Embeddings[0]) != 384 {
			return nil, fmt.Errorf("MiniLM returned an invalid embedding shape")
		}
		vectors[position] = make([]float64, 384)
		for dimension, value := range result.Embeddings[0] {
			vectors[position][dimension] = float64(value)
		}
	}
	return vectors, nil
}

// Split uses the same untruncated tokenizer as inference. The encoder's gate
// prevents teardown or another use of the mutable library tokenizer mid-split.
func (encoder *Encoder) Split(operationContext context.Context, text string) ([]string, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	select {
	case encoder.gate <- struct{}{}:
	case <-operationContext.Done():
		return nil, operationContext.Err()
	}
	defer func() { <-encoder.gate }()
	if encoder.closed {
		return nil, ErrClosed
	}
	return splitTextWithinTokenLimit(operationContext, text, encoder.countWordPieceTokens)
}

func (encoder *Encoder) countWordPieceTokens(text string) int {
	return len(encoder.pipeline.GetModel().Tokenizer.GoTokenizer.Tokenizer.Encode(text))
}

func (encoder *Encoder) Close() error {
	encoder.gate <- struct{}{}
	defer func() { <-encoder.gate }()
	if encoder.closed {
		return encoder.closeError
	}
	encoder.closed = true
	encoder.closeError = encoder.session.Destroy()
	return encoder.closeError
}
