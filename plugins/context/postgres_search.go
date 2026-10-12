package contextplugin

import (
	"context"
	"fmt"
	"strings"
)

const minimumSearchCandidates = 64
const maximumSearchCandidates = 1024

const searchScope = `workspace_id=$1 AND kind=ANY($2) AND ($3 OR NOT deleted) AND ($4 OR NOT historical)
 AND (cardinality($5::text[])=0 OR categories && $5)
 AND ($6='' OR model=$6) AND ($7=0 OR vector_dims(embedding)=$7)`

func (database *postgresRepository) Search(operationContext context.Context, workspaceID string, query searchQuery) ([]searchHit, error) {
	if query.Limit < 1 || query.Limit > 100 {
		return nil, fmt.Errorf("memory search limit is outside 1..100")
	}
	if len(query.Vector) > 0 && query.EmbeddingModel == "" {
		return nil, fmt.Errorf("vector search requires an embedding model")
	}
	vector := "[0]"
	var operationError error
	if len(query.Vector) > 0 {
		vector, operationError = encodeVector(query.Vector)
		if operationError != nil {
			return nil, operationError
		}
	}
	if query.Categories == nil {
		query.Categories = []string{}
	}
	if len(query.Kinds) == 0 {
		query.Kinds = []string{"memory"}
	}
	if operationError = database.ensureSearchIndex(operationContext, query.EmbeddingModel, len(query.Vector)); operationError != nil {
		return nil, operationError
	}
	transaction, operationError := database.pool.Begin(operationContext)
	if operationError != nil {
		return nil, operationError
	}
	defer transaction.Rollback(context.WithoutCancel(operationContext))
	// Parameter-sensitive partial indexes need a custom plan. Iterative HNSW
	// scans continue past rows from other workspaces, versions or categories.
	candidateLimit := min(max(query.Limit*16, minimumSearchCandidates), maximumSearchCandidates)
	settings := fmt.Sprintf(`SET LOCAL plan_cache_mode=force_custom_plan; SET LOCAL hnsw.iterative_scan=strict_order; SET LOCAL hnsw.ef_search=%d`, min(candidateLimit, 1000))
	if _, operationError = transaction.Exec(operationContext, settings); operationError != nil {
		return nil, operationError
	}
	literal := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.ToLower(query.Text)) + "%"
	arguments := []any{workspaceID, query.Kinds, query.IncludeDeleted, query.IncludeHistory, query.Categories, query.EmbeddingModel, len(query.Vector), vector, query.Text, query.Limit, candidateLimit, literal}
	readHits := func(exact bool) ([]searchHit, int, error) {
		rows, operationError := transaction.Query(operationContext, memorySearchSQL(len(query.Vector), exact), arguments...)
		if operationError != nil {
			return nil, 0, operationError
		}
		defer rows.Close()
		var hits []searchHit
		var nearestDocuments int
		for rows.Next() {
			var hit searchHit
			if operationError := rows.Scan(&hit.Kind, &hit.ID, &hit.Version, &hit.Text, &hit.Score, &nearestDocuments); operationError != nil {
				return nil, 0, operationError
			}
			hits = append(hits, hit)
		}
		return hits, nearestDocuments, rows.Err()
	}
	hits, nearestDocuments, operationError := readHits(false)
	if operationError != nil {
		return nil, operationError
	}
	if len(query.Vector) > 0 && nearestDocuments < query.Limit {
		// Restrictive filters or a many-chunk document can exhaust ANN candidates.
		// An exact, document-deduplicated fallback prevents those cases from hiding
		// other eligible documents. Ordinary queries rerank only bounded candidates.
		hits, _, operationError = readHits(true)
		if operationError != nil {
			return nil, operationError
		}
	}
	if operationError = transaction.Commit(operationContext); operationError != nil {
		return nil, operationError
	}
	return hits, nil
}

func memorySearchSQL(dimensions int, exact bool) string {
	distance := `(embedding <=> $8::vector)`
	nearest := `SELECT kind,key,version,chunk,content,embedding,search_text FROM context_plugin.vectors WHERE false`
	if dimensions > 0 {
		distance = fmt.Sprintf(`(embedding::vector(%d) <=> $8::vector(%d))`, dimensions, dimensions)
		nearest = `SELECT kind,key,version,chunk,content,embedding,search_text FROM context_plugin.vectors WHERE ` + searchScope + ` ORDER BY ` + distance + ` LIMIT $11`
		if exact {
			nearest = `SELECT kind,key,version,chunk,content,embedding,search_text FROM (
    SELECT DISTINCT ON(kind,key,version) kind,key,version,chunk,content,embedding,search_text,` + distance + ` AS distance
    FROM context_plugin.vectors WHERE ` + searchScope + ` ORDER BY kind,key,version,` + distance + `,chunk
   ) documents ORDER BY distance,kind,key,version DESC LIMIT $11`
		}
	}
	return `WITH nearest AS MATERIALIZED (` + nearest + `), lexical AS MATERIALIZED (
  SELECT kind,key,version,chunk,content,embedding,search_text FROM (
  SELECT DISTINCT ON(kind,key,version) kind,key,version,chunk,content,embedding,search_text,
   ts_rank_cd(search_text,plainto_tsquery('simple',$9)) AS lexical_rank FROM context_plugin.vectors
  WHERE ` + searchScope + ` AND $9<>'' AND search_text @@ plainto_tsquery('simple',$9)
  ORDER BY kind,key,version,lexical_rank DESC,chunk
  ) documents ORDER BY lexical_rank DESC,kind,key,version DESC LIMIT $11
 ), literal AS MATERIALIZED (
  SELECT DISTINCT ON(kind,key,version) kind,key,version,chunk,content,embedding,search_text FROM context_plugin.vectors
  WHERE ` + searchScope + ` AND $9<>'' AND lower(content) LIKE $12
  ORDER BY kind,key,version DESC,chunk LIMIT $11
 ), browse AS MATERIALIZED (
  SELECT DISTINCT ON(kind,key,version) kind,key,version,chunk,content,embedding,search_text FROM context_plugin.vectors
  WHERE ` + searchScope + ` AND $9='' AND $7=0 ORDER BY kind,key,version DESC,chunk LIMIT $11
 ), candidates AS (
  SELECT * FROM nearest UNION ALL SELECT * FROM lexical UNION ALL SELECT * FROM literal UNION ALL SELECT * FROM browse
 ), ranked AS (
  SELECT kind,key,version,content,
   (CASE WHEN $7>0 THEN GREATEST(0,1-` + distance + `) ELSE 0 END)
   + (CASE WHEN $9<>'' THEN ts_rank_cd(search_text,plainto_tsquery('simple',$9)) ELSE 0 END)
   + (CASE WHEN $9<>'' AND strpos(lower(content),lower($9))>0 THEN 1 ELSE 0 END) AS score FROM candidates
 ), best AS (
  SELECT DISTINCT ON(kind,key,version) kind,key,version,content,score FROM ranked ORDER BY kind,key,version,score DESC,content
 ) SELECT kind,key,version,content,score,(SELECT COUNT(DISTINCT (kind,key,version)) FROM nearest)
 FROM best WHERE $9='' OR score>0.15 ORDER BY score DESC,kind,key,version DESC LIMIT $10`
}
