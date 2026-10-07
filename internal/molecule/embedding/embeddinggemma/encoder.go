//go:build gemma && cgo

package embeddinggemma

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/daulet/tokenizers"
	ort "github.com/yalue/onnxruntime_go"
)

var ErrClosed = errors.New("EmbeddingGemma encoder is closed")

// Encoder serializes tokenizer/inference access and joins active work on Close.
// It loads only the shared text graph. Media feature inputs are empty tensors.
type Encoder struct {
	configuration Configuration
	tokenizer     *tokenizers.Tokenizer
	session       *ort.DynamicAdvancedSession
	gate          chan struct{}
	closed        bool
}

func New(configuration Configuration) (*Encoder, error) {
	if operationError := configuration.validate(); operationError != nil {
		return nil, operationError
	}
	data, operationError := os.ReadFile(filepath.Join(configuration.ModelDirectory, "tokenizer.json"))
	if operationError != nil {
		return nil, operationError
	}
	var settings struct {
		Truncation json.RawMessage `json:"truncation"`
		Padding    json.RawMessage `json:"padding"`
	}
	if operationError := json.Unmarshal(data, &settings); operationError != nil {
		return nil, operationError
	}
	for _, option := range []json.RawMessage{settings.Truncation, settings.Padding} {
		if len(option) > 0 && string(option) != "null" {
			return nil, fmt.Errorf("EmbeddingGemma tokenizer has automatic truncation or padding enabled")
		}
	}
	// Treat literal media markers in source text as text, not media activation.
	// BOS/EOS are still inserted by the tokenizer's postprocessor.
	tokenizer, operationError := tokenizers.FromBytes(data, tokenizers.WithEncodeSpecialTokens())
	if operationError != nil {
		return nil, operationError
	}
	if operationError := acquireEnvironment(configuration.RuntimeLibrary); operationError != nil {
		return nil, errors.Join(operationError, tokenizer.Close())
	}
	options, operationError := ort.NewSessionOptions()
	if operationError != nil {
		return nil, errors.Join(operationError, tokenizer.Close(), releaseEnvironment())
	}
	operationError = errors.Join(options.SetIntraOpNumThreads(configuration.Threads), options.SetInterOpNumThreads(1))
	if operationError != nil {
		return nil, errors.Join(operationError, options.Destroy(), tokenizer.Close(), releaseEnvironment())
	}
	session, operationError := ort.NewDynamicAdvancedSession(filepath.Join(configuration.ModelDirectory, configuration.ModelFile), []string{"input_ids", "attention_mask", "image_features", "video_features", "audio_features"}, []string{"sentence_embedding"}, options)
	operationError = errors.Join(operationError, options.Destroy())
	if operationError != nil {
		if session != nil {
			operationError = errors.Join(operationError, session.Destroy())
		}
		return nil, errors.Join(operationError, tokenizer.Close(), releaseEnvironment())
	}
	return &Encoder{configuration: configuration, tokenizer: tokenizer, session: session, gate: make(chan struct{}, 1)}, nil
}

// Split reserves the larger query/document prefix budget and keeps every byte.
func (encoder *Encoder) Split(operationContext context.Context, text string) ([]string, error) {
	if operationError := encoder.acquire(operationContext); operationError != nil {
		return nil, operationError
	}
	defer encoder.release()
	return splitText(operationContext, text, encoder.configuration.ChunkTokens, func(fragment string) (int, error) {
		document, operationError := encoder.tokenize(documentPrefix + fragment)
		if operationError != nil {
			return 0, operationError
		}
		query, operationError := encoder.tokenize(queryPrefix + fragment)
		return max(len(document), len(query)), operationError
	})
}

// Embed encodes stored documents with Google's retrieval-document prefix.
func (encoder *Encoder) Embed(operationContext context.Context, texts []string) ([][]float64, error) {
	return encoder.embed(operationContext, texts, documentPrefix)
}

// EmbedQueries uses the matching retrieval-query prefix in the same space.
func (encoder *Encoder) EmbedQueries(operationContext context.Context, texts []string) ([][]float64, error) {
	return encoder.embed(operationContext, texts, queryPrefix)
}

func (encoder *Encoder) embed(operationContext context.Context, texts []string, prefix string) ([][]float64, error) {
	if operationError := encoder.acquire(operationContext); operationError != nil {
		return nil, operationError
	}
	defer encoder.release()
	vectors := make([][]float64, 0, len(texts))
	for _, text := range texts {
		if operationError := operationContext.Err(); operationError != nil {
			return nil, operationError
		}
		tokens, operationError := encoder.tokenize(prefix + text)
		if operationError != nil {
			return nil, operationError
		}
		if len(tokens) > maxTokens {
			return nil, fmt.Errorf("EmbeddingGemma input has %d tokens; maximum is %d", len(tokens), maxTokens)
		}
		vector, operationError := encoder.runInference(operationContext, tokens)
		if operationError != nil {
			return nil, operationError
		}
		vectors = append(vectors, vector)
	}
	return vectors, nil
}

func (encoder *Encoder) tokenize(text string) ([]uint32, error) {
	if !utf8.ValidString(text) {
		return nil, fmt.Errorf("EmbeddingGemma input is not valid UTF-8")
	}
	if strings.IndexByte(text, 0) >= 0 {
		return nil, fmt.Errorf("EmbeddingGemma input contains a NUL byte")
	}
	encoding, operationError := encoder.tokenizer.EncodeWithOptionsErr(text, true)
	if operationError != nil {
		return nil, operationError
	}
	if len(encoding.IDs) < 2 {
		return nil, fmt.Errorf("EmbeddingGemma tokenizer returned no complete token sequence")
	}
	return encoding.IDs, nil
}

func (encoder *Encoder) acquire(operationContext context.Context) error {
	if operationError := operationContext.Err(); operationError != nil {
		return operationError
	}
	select {
	case encoder.gate <- struct{}{}:
	case <-operationContext.Done():
		return operationContext.Err()
	}
	if encoder.closed {
		encoder.release()
		return ErrClosed
	}
	if operationError := operationContext.Err(); operationError != nil {
		encoder.release()
		return operationError
	}
	return nil
}

func (encoder *Encoder) release() { <-encoder.gate }

func (encoder *Encoder) Close() error {
	encoder.gate <- struct{}{}
	defer encoder.release()
	if encoder.closed {
		return nil
	}
	encoder.closed = true
	return errors.Join(encoder.session.Destroy(), encoder.tokenizer.Close(), releaseEnvironment())
}
