package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type ownedTextEmbedder interface {
	harness.TextEmbedder
	Close() error
}

// lazyPluginEmbeddings owns one inference resource shared by memory workers.
// Tool discovery retains its independent optional backend and fallback policy.
type lazyPluginEmbeddings struct {
	mutex   sync.Mutex
	create  func() (ownedTextEmbedder, error)
	encoder ownedTextEmbedder
	closed  bool
}

func (service *lazyPluginEmbeddings) load(operationContext context.Context) (ownedTextEmbedder, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	if service.closed {
		return nil, fmt.Errorf("plugin embedding service is closed")
	}
	if service.encoder == nil {
		encoder, operationError := service.create()
		if operationError != nil {
			return nil, operationError
		}
		service.encoder = encoder
	}
	return service.encoder, nil
}
func (service *lazyPluginEmbeddings) Split(operationContext context.Context, text string) ([]string, error) {
	encoder, operationError := service.load(operationContext)
	if operationError != nil {
		return nil, operationError
	}
	return encoder.Split(operationContext, text)
}
func (service *lazyPluginEmbeddings) Embed(operationContext context.Context, text []string) ([][]float64, error) {
	encoder, operationError := service.load(operationContext)
	if operationError != nil {
		return nil, operationError
	}
	return encoder.Embed(operationContext, text)
}

func (service *lazyPluginEmbeddings) EmbedQueries(operationContext context.Context, text []string) ([][]float64, error) {
	encoder, operationError := service.load(operationContext)
	if operationError != nil {
		return nil, operationError
	}
	if queryEncoder, available := encoder.(harness.QueryTextEmbedder); available {
		return queryEncoder.EmbedQueries(operationContext, text)
	}
	return encoder.Embed(operationContext, text)
}
func (service *lazyPluginEmbeddings) Close() error {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if service.closed {
		return nil
	}
	service.closed = true
	if service.encoder != nil {
		return service.encoder.Close()
	}
	return nil
}

func embeddingAssetIdentity(directory string) (string, error) {
	digest := sha256.New()
	for _, name := range []string{"model.onnx", "tokenizer.json", "config.json", "sentence_bert_config.json"} {
		file, operationError := os.Open(filepath.Join(directory, name))
		if operationError != nil {
			return "", operationError
		}
		_, copyError := io.Copy(digest, file)
		closeError := file.Close()
		if copyError != nil {
			return "", copyError
		}
		if closeError != nil {
			return "", closeError
		}
	}
	return "sha256:" + hex.EncodeToString(digest.Sum(nil)), nil
}
