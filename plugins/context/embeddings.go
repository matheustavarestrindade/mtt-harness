package contextplugin

import (
	"context"
	"fmt"
	"math"
	"time"
)

func (plugin *Plugin) embedText(operationContext context.Context, workspaceID, agent, text string) ([]vectorChunk, error) {
	if plugin.services.Embeddings == nil {
		return nil, fmt.Errorf("semantic embeddings are unavailable")
	}
	started := time.Now()
	chunks, operationError := plugin.services.Embeddings.Split(operationContext, text)
	if operationError != nil {
		return nil, operationError
	}
	if len(chunks) == 0 {
		return nil, fmt.Errorf("embedding tokenizer returned no chunks")
	}
	result := make([]vectorChunk, 0, len(chunks))
	for _, chunk := range chunks {
		if operationError := operationContext.Err(); operationError != nil {
			return nil, operationError
		}
		vectors, operationError := plugin.services.Embeddings.Embed(operationContext, []string{chunk})
		if operationError != nil {
			return nil, operationError
		}
		if len(vectors) != 1 {
			return nil, fmt.Errorf("embedding service returned %d vectors for one chunk", len(vectors))
		}
		if _, operationError := encodeVector(vectors[0]); operationError != nil {
			return nil, operationError
		}
		result = append(result, vectorChunk{Text: chunk, Vector: vectors[0]})
	}
	if operationError := plugin.database.Metric(operationContext, workspaceID, agent, "embedding_chunks", int64(len(chunks)), time.Since(started).Milliseconds()); operationError != nil {
		return nil, operationError
	}
	return result, nil
}

func queryVector(chunks []vectorChunk) ([]float64, error) {
	if len(chunks) == 0 {
		return nil, fmt.Errorf("query has no embedding chunks")
	}
	result := make([]float64, len(chunks[0].Vector))
	for _, chunk := range chunks {
		if len(chunk.Vector) != len(result) {
			return nil, fmt.Errorf("query embedding dimensions differ")
		}
		for index, value := range chunk.Vector {
			result[index] += value
		}
	}
	norm := 0.0
	for _, value := range result {
		norm += value * value
	}
	if norm == 0 {
		return nil, fmt.Errorf("query embedding has zero norm")
	}
	norm = math.Sqrt(norm)
	for index := range result {
		result[index] /= norm
	}
	return result, nil
}
