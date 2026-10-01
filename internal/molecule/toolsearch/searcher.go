package toolsearch

import "context"

// Searcher ranks internal tool documents. Implementations own only derived
// search data; the runtime registry remains authoritative for callable tools.
// Documents, vectors, and scores are never model-facing discovery results.
type Searcher interface {
	Search(operationContext context.Context, documents []Document, query string) ([]Match, error)
}

// Match identifies an internal ranked document. The registry resolves the ID
// and sends compact tool references rather than the vector or score.
type Match struct {
	ID         string
	Similarity float64
}
