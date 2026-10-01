package toolsearch

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"math"

	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/embedding"
)

type Options struct {
	DocumentPrefix    string
	QueryPrefix       string
	MinimumSimilarity float64
}

type indexedDocument struct {
	fingerprint [32]byte
	vectors     [][]float64
}

type inputRange struct {
	start int
	end   int
}

// Index owns derived chunk vectors behind a cancellable gate. A tool's score is
// its best matching chunk, so a parameter late in a long schema stays searchable.
// Tool registrations remain authoritative outside this cache.
type Index struct {
	embedder embedding.Embedder
	options  Options
	gate     chan struct{}
	cache    map[string]indexedDocument
}

func New(embedder embedding.Embedder, options Options) *Index {
	return &Index{embedder: embedder, options: options, gate: make(chan struct{}, 1), cache: map[string]indexedDocument{}}
}

func (index *Index) Search(operationContext context.Context, documents []Document, query string) ([]Match, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	if len(documents) == 0 {
		return nil, nil
	}
	if index.embedder == nil {
		return nil, fmt.Errorf("tool search: embedding backend is not configured")
	}
	select {
	case index.gate <- struct{}{}:
	case <-operationContext.Done():
		return nil, operationContext.Err()
	}
	defer func() { <-index.gate }()
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	vectors := make([][][]float64, len(documents))
	fingerprints := make([][32]byte, len(documents))
	changed := make(map[int]inputRange)
	var inputs []string
	for position, document := range documents {
		text := index.options.DocumentPrefix + document.Text
		fingerprints[position] = sha256.Sum256([]byte(text))
		cached, found := index.cache[document.ID]
		if found && cached.fingerprint == fingerprints[position] {
			vectors[position] = cached.vectors
			continue
		}
		chunks, operationError := index.split(operationContext, text)
		if operationError != nil {
			return nil, operationError
		}
		changed[position] = inputRange{start: len(inputs), end: len(inputs) + len(chunks)}
		inputs = append(inputs, chunks...)
	}
	queryChunks, operationError := index.split(operationContext, index.options.QueryPrefix+query)
	if operationError != nil {
		return nil, operationError
	}
	queryStart := len(inputs)
	inputs = append(inputs, queryChunks...)
	embeddings, operationError := index.embedder.Embed(operationContext, inputs)
	if operationError != nil {
		return nil, fmt.Errorf("tool search: %w", operationError)
	}
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	if len(embeddings) != len(inputs) {
		return nil, fmt.Errorf("tool search: embedding count does not match input count")
	}
	for position := range embeddings {
		embeddings[position], operationError = normalize(embeddings[position])
		if operationError != nil {
			return nil, operationError
		}
	}
	for position, selected := range changed {
		vectors[position] = embeddings[selected.start:selected.end]
	}
	var matches []Match
	for position, document := range documents {
		if operationError := operationContext.Err(); operationError != nil {
			return nil, operationError
		}
		best := -1.0
		for _, documentVector := range vectors[position] {
			for _, queryVector := range embeddings[queryStart:] {
				if len(documentVector) != len(queryVector) {
					return nil, fmt.Errorf("tool search: embedding dimensions changed for %s", document.ID)
				}
				similarity := 0.0
				for dimension, value := range queryVector {
					similarity += value * documentVector[dimension]
				}
				best = max(best, similarity)
			}
		}
		if best >= index.options.MinimumSimilarity {
			matches = append(matches, Match{ID: document.ID, Similarity: best})
		}
	}
	active := map[string]bool{}
	for position, document := range documents {
		active[document.ID] = true
		index.cache[document.ID] = indexedDocument{fingerprint: fingerprints[position], vectors: vectors[position]}
	}
	for identifier := range index.cache {
		if !active[identifier] {
			delete(index.cache, identifier)
		}
	}
	sortMatches(matches)
	return matches, nil
}

func (index *Index) split(operationContext context.Context, text string) ([]string, error) {
	if splitter, available := index.embedder.(embedding.Splitter); available {
		chunks, operationError := splitter.Split(operationContext, text)
		if operationError != nil {
			return nil, operationError
		}
		if len(chunks) == 0 {
			return nil, fmt.Errorf("tool search: tokenizer returned no chunks")
		}
		return chunks, nil
	}
	return []string{text}, nil
}

// Close follows encoder ownership after the selector joins active searches.
func (index *Index) Close() error {
	index.gate <- struct{}{}
	defer func() { <-index.gate }()
	if closer, available := index.embedder.(io.Closer); available {
		return closer.Close()
	}
	return nil
}

func normalize(vector []float64) ([]float64, error) {
	magnitude := 0.0
	for _, value := range vector {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, fmt.Errorf("tool search: non-finite embedding")
		}
		magnitude += value * value
	}
	if magnitude == 0 || math.IsInf(magnitude, 0) {
		return nil, fmt.Errorf("tool search: empty or invalid embedding")
	}
	magnitude = math.Sqrt(magnitude)
	normalized := make([]float64, len(vector))
	for dimension, value := range vector {
		normalized[dimension] = value / magnitude
	}
	return normalized, nil
}
