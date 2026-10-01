package toolsearch

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math"
	"sort"
	"strings"
	"unicode"
)

// Lexical is a non-neural TF-IDF/cosine backend over the same full documents.
// Its corpus cache is gate-owned. Any catalog change rebuilds IDF weights so
// removed or changed MCP tools cannot retain a stale vocabulary or score.
type Lexical struct {
	minimum     float64
	gate        chan struct{}
	fingerprint [32]byte
	ready       bool
	idf         map[string]float64
	vectors     []lexicalDocument
}

type lexicalDocument struct {
	identifier string
	vector     map[string]float64
}

func NewLexical(minimumSimilarity float64) *Lexical {
	return &Lexical{minimum: minimumSimilarity, gate: make(chan struct{}, 1)}
}

func (lexical *Lexical) Search(operationContext context.Context, documents []Document, query string) ([]Match, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	select {
	case lexical.gate <- struct{}{}:
	case <-operationContext.Done():
		return nil, operationContext.Err()
	}
	defer func() { <-lexical.gate }()
	fingerprint := corpusFingerprint(documents)
	if !lexical.ready || lexical.fingerprint != fingerprint {
		if operationError := lexical.rebuild(operationContext, documents); operationError != nil {
			return nil, operationError
		}
		lexical.fingerprint, lexical.ready = fingerprint, true
	}
	queryVector := lexical.weight(termCounts(query))
	if len(queryVector) == 0 {
		return nil, nil
	}
	var matches []Match
	queryTerms := sortedTerms(queryVector)
	for _, document := range lexical.vectors {
		if operationError := operationContext.Err(); operationError != nil {
			return nil, operationError
		}
		similarity := 0.0
		// Sorting query terms makes floating-point accumulation reproducible.
		for _, term := range queryTerms {
			similarity += queryVector[term] * document.vector[term]
		}
		if similarity > 0 && similarity >= lexical.minimum {
			matches = append(matches, Match{ID: document.identifier, Similarity: similarity})
		}
	}
	sortMatches(matches)
	return matches, nil
}

func (lexical *Lexical) rebuild(operationContext context.Context, documents []Document) error {
	frequencies := map[string]int{}
	counts := make([]map[string]float64, len(documents))
	for position, document := range documents {
		if operationError := operationContext.Err(); operationError != nil {
			return operationError
		}
		counts[position] = termCounts(document.Text)
		for term := range counts[position] {
			frequencies[term]++
		}
	}
	lexical.idf = make(map[string]float64, len(frequencies))
	for term, frequency := range frequencies {
		// Add one after smoothing so a one-document catalog stays searchable.
		lexical.idf[term] = math.Log(float64(len(documents)+1)/float64(frequency+1)) + 1
	}
	lexical.vectors = make([]lexicalDocument, len(documents))
	for position, document := range documents {
		vector := lexical.weight(counts[position])
		if document.Summary != "" {
			// The full schema remains searchable. Primary capability metadata
			// receives more weight than an example mentioning another tool.
			summary := lexical.weight(termCounts(document.Summary))
			for term := range vector {
				vector[term] *= 0.25
			}
			for term, value := range summary {
				vector[term] += 0.75 * value
			}
			normalizeTerms(vector)
		}
		lexical.vectors[position] = lexicalDocument{identifier: document.ID, vector: vector}
	}
	return nil
}

func (lexical *Lexical) weight(counts map[string]float64) map[string]float64 {
	vector := make(map[string]float64, len(counts))
	magnitude := 0.0
	for _, term := range sortedTerms(counts) {
		inverseFrequency, present := lexical.idf[term]
		if !present {
			continue
		}
		weight := (1 + math.Log(counts[term])) * inverseFrequency
		vector[term] = weight
		magnitude += weight * weight
	}
	if magnitude == 0 {
		return nil
	}
	for term := range vector {
		vector[term] /= math.Sqrt(magnitude)
	}
	return vector
}

func termCounts(text string) map[string]float64 {
	counts := map[string]float64{}
	for _, term := range strings.FieldsFunc(strings.ToLower(text), func(character rune) bool { return !unicode.IsLetter(character) && !unicode.IsDigit(character) }) {
		counts[term]++
	}
	return counts
}

func sortedTerms(vector map[string]float64) []string {
	terms := make([]string, 0, len(vector))
	for term := range vector {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	return terms
}

func corpusFingerprint(documents []Document) [32]byte {
	hasher := sha256.New()
	var length [8]byte
	for _, document := range documents {
		for _, field := range []string{document.ID, document.Text, document.Summary} {
			binary.LittleEndian.PutUint64(length[:], uint64(len(field)))
			hasher.Write(length[:])
			hasher.Write([]byte(field))
		}
	}
	var fingerprint [32]byte
	copy(fingerprint[:], hasher.Sum(nil))
	return fingerprint
}

func normalizeTerms(vector map[string]float64) {
	magnitude := 0.0
	for _, term := range sortedTerms(vector) {
		magnitude += vector[term] * vector[term]
	}
	if magnitude == 0 {
		return
	}
	for term := range vector {
		vector[term] /= math.Sqrt(magnitude)
	}
}
