package contextplugin

import "context"

func (database *postgresRepository) Search(operationContext context.Context, workspaceID string, query searchQuery) ([]searchHit, error) {
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
	// Filtering precedes ranking. Exact text contributes independently of
	// semantic similarity; consolidated detail remains searchable by default.
	rows, operationError := database.pool.Query(operationContext, `
	 WITH candidates AS MATERIALIZED (
	   SELECT kind,key,version,content,embedding,search_text FROM context_plugin.vectors
	   WHERE workspace_id=$1 AND kind=ANY($2) AND ($3 OR NOT deleted) AND ($4 OR NOT historical)
	     AND (cardinality($5::text[])=0 OR categories && $5)
	     AND ($6='' OR model=$6) AND ($7=0 OR vector_dims(embedding)=$7)
	 ), ranked AS (
	   SELECT kind,key,version,content,
	     (CASE WHEN $7>0 THEN GREATEST(0,1-(embedding <=> $8::vector)) ELSE 0 END)
	     + (CASE WHEN $9<>'' THEN ts_rank_cd(search_text,plainto_tsquery('simple',$9)) ELSE 0 END)
	     + (CASE WHEN $9<>'' AND strpos(lower(content),lower($9))>0 THEN 1 ELSE 0 END) AS score
	   FROM candidates
	 ), best AS (SELECT DISTINCT ON(kind,key,version) kind,key,version,content,score FROM ranked ORDER BY kind,key,version,score DESC)
	 SELECT kind,key,version,content,score FROM best WHERE $9='' OR score>0.15 ORDER BY score DESC,kind,key,version DESC LIMIT $10`, workspaceID, query.Kinds, query.IncludeDeleted, query.IncludeHistory, query.Categories, query.EmbeddingModel, len(query.Vector), vector, query.Text, query.Limit)
	if operationError != nil {
		return nil, operationError
	}
	defer rows.Close()
	var hits []searchHit
	for rows.Next() {
		var hit searchHit
		if operationError := rows.Scan(&hit.Kind, &hit.ID, &hit.Version, &hit.Text, &hit.Score); operationError != nil {
			return nil, operationError
		}
		hits = append(hits, hit)
	}
	return hits, rows.Err()
}
