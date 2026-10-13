package contextplugin

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func isolatedRepository(test *testing.T) (*postgresRepository, string) {
	test.Helper()
	databaseURL := os.Getenv("MTT_TEST_DATABASE_URL")
	if databaseURL == "" {
		test.Skip("MTT_TEST_DATABASE_URL is not configured")
	}
	database, operationError := openRepository(context.Background(), databaseURL)
	testutil.RequireNoError(test, operationError)
	workspaceID := "performance-" + newIdentifier()
	test.Cleanup(func() {
		_, _ = database.pool.Exec(context.Background(), `DELETE FROM context_plugin.vectors WHERE workspace_id LIKE $1`, workspaceID+"%")
		_, _ = database.pool.Exec(context.Background(), `DELETE FROM context_plugin.documents WHERE workspace_id LIKE $1`, workspaceID+"%")
		database.Close()
	})
	return database, workspaceID
}

func TestJobClaimFiltersTerminalHistoryAndHonorsPriorityLeasesAndRetryTime(test *testing.T) {
	database, workspaceID := isolatedRepository(test)
	operationContext := context.Background()
	_, operationError := database.pool.Exec(operationContext, `INSERT INTO context_plugin.documents(workspace_id,kind,key,data)
 SELECT $1,'job','finished-'||position,jsonb_build_object('id','finished-'||position,'status','done','priority',0) FROM generate_series(1,5000) position`, workspaceID)
	testutil.RequireNoError(test, operationError)
	now := time.Now().UTC()
	jobs := []memoryJob{
		{ID: "normal", Status: "pending", Priority: 10, CreatedAt: now.Add(-time.Hour)},
		{ID: "first", Status: "pending", Priority: 1, CreatedAt: now.Add(-time.Minute)},
		{ID: "expired", Status: "running", Priority: 2, CreatedAt: now, LeaseUntil: now.Add(-time.Minute)},
		{ID: "leased", Status: "running", Priority: 0, CreatedAt: now, LeaseUntil: now.Add(time.Hour)},
		{ID: "retry-later", Status: "pending", Priority: 0, CreatedAt: now, NextAttempt: now.Add(time.Hour)},
	}
	testutil.RequireNoError(test, database.Transact(operationContext, workspaceID, func(transaction transaction) error {
		for _, job := range jobs {
			if operationError := transaction.Put("job", job.ID, job); operationError != nil {
				return operationError
			}
		}
		selected, operationError := transaction.RunnableJobs(now, 2)
		if operationError != nil {
			return operationError
		}
		if len(selected) != 2 || selected[0].ID != "first" || selected[1].ID != "expired" {
			test.Fatalf("job selection = %+v", selected)
		}
		active, operationError := transaction.HasActiveJobs()
		if !active {
			test.Fatal("active jobs disappeared")
		}
		return operationError
	}))
	plugin := &Plugin{database: database}
	selected, found, operationError := plugin.claimJob(operationContext, workspaceID, configuration{JobTimeoutMilliseconds: 1000})
	testutil.RequireNoError(test, operationError)
	if !found || selected.ID != "first" || selected.Lease == "" || selected.Attempts != 1 {
		test.Fatalf("claim = %+v", selected)
	}
}

func TestWorkspaceDispatchPagesProgressBeyondTheFirstPage(test *testing.T) {
	database, workspaceID := isolatedRepository(test)
	for index := 0; index < maximumWorkspaceDispatchPage+3; index++ {
		identifier := fmt.Sprintf("%s-%03d", workspaceID, index)
		testutil.RequireNoError(test, database.Transact(context.Background(), identifier, func(transaction transaction) error { return transaction.Put("workspace", "state", map[string]any{}) }))
	}
	after := workspaceID
	seen := map[string]bool{}
	for {
		workspaces, operationError := database.PendingWorkspaces(context.Background(), after, maximumWorkspaceDispatchPage)
		testutil.RequireNoError(test, operationError)
		if len(workspaces) == 0 {
			break
		}
		for _, identifier := range workspaces {
			after = identifier
			if !strings.HasPrefix(identifier, workspaceID) {
				continue
			}
			if seen[identifier] {
				test.Fatal("workspace pagination repeated an entry")
			}
			seen[identifier] = true
		}
		if len(seen) == maximumWorkspaceDispatchPage+3 {
			break
		}
	}
	if len(seen) != maximumWorkspaceDispatchPage+3 {
		test.Fatalf("visited %d workspaces", len(seen))
	}
}

func TestIndexedHybridRetrievalKeepsFiltersVersionsAndDocumentDiversity(test *testing.T) {
	database, workspaceID := isolatedRepository(test)
	operationContext := context.Background()
	model := "audit-vector-space"
	insert := func(workspace, kind, key, space, text, vector string, chunk int, deleted, historical bool) {
		test.Helper()
		_, operationError := database.pool.Exec(operationContext, `INSERT INTO context_plugin.vectors(workspace_id,kind,key,version,chunk,model,categories,content,embedding,deleted,historical) VALUES($1,$2,$3,1,$4,$5,ARRAY['architecture'],$6,$7::vector,$8,$9)`, workspace, kind, key, chunk, space, text, vector, deleted, historical)
		testutil.RequireNoError(test, operationError)
	}
	insert(workspaceID, "memory", "needle", model, "PostgreSQL durable records", "[0,1]", 0, false, false)
	insert(workspaceID, "memory", "literal", model, "identifier %_literal is exact", "[0.1,0.9]", 0, false, false)
	insert(workspaceID, "memory", "deleted", model, "PostgreSQL secret deleted", "[0,1]", 0, true, false)
	insert(workspaceID, "memory", "old", model, "PostgreSQL old version", "[0,1]", 0, false, true)
	insert(workspaceID, "memory", "other-space", "different-space", "PostgreSQL wrong model", "[0,1]", 0, false, false)
	insert(workspaceID, "memory", "other-dimensions", model, "PostgreSQL wrong dimensions", "[0,1,0]", 0, false, false)
	insert(workspaceID+"-other", "memory", "foreign", model, "PostgreSQL another workspace", "[0,1]", 0, false, false)
	_, operationError := database.pool.Exec(operationContext, `INSERT INTO context_plugin.vectors(workspace_id,kind,key,version,chunk,model,categories,content,embedding)
 SELECT $1,'memory','large',1,position,$2,ARRAY['architecture'],'PostgreSQL large document','[0,1]'::vector FROM generate_series(0,300) position`, workspaceID, model)
	testutil.RequireNoError(test, operationError)
	query := searchQuery{Text: "PostgreSQL", Vector: []float64{0, 1}, EmbeddingModel: model, Categories: []string{"architecture"}, Limit: 3}
	hits, operationError := database.Search(operationContext, workspaceID, query)
	testutil.RequireNoError(test, operationError)
	seen := map[string]bool{}
	for _, hit := range hits {
		if seen[hit.ID] {
			test.Fatal("duplicate memory chunks escaped deduplication")
		}
		seen[hit.ID] = true
	}
	if len(hits) != 3 || !seen["needle"] || !seen["large"] || !seen["literal"] {
		test.Fatalf("filtered hybrid results: %+v", hits)
	}
	query.Categories = []string{"absent"}
	hits, operationError = database.Search(operationContext, workspaceID, query)
	testutil.RequireNoError(test, operationError)
	if len(hits) != 0 {
		test.Fatal("category filter leaked content")
	}
	hits, operationError = database.Search(operationContext, workspaceID, searchQuery{Text: "%_literal", EmbeddingModel: model, Limit: 3})
	testutil.RequireNoError(test, operationError)
	if len(hits) != 1 || hits[0].ID != "literal" {
		test.Fatalf("literal wildcard escaping: %+v", hits)
	}
	hits, operationError = database.Search(operationContext, workspaceID, searchQuery{Categories: []string{"architecture"}, EmbeddingModel: model, Limit: 4})
	testutil.RequireNoError(test, operationError)
	if len(hits) != 4 {
		test.Fatalf("category browsing lost document diversity: %+v", hits)
	}
	// The retained data stays queryable when historical/deleted records are explicit.
	query.Categories = nil
	query.IncludeDeleted = true
	query.IncludeHistory = true
	query.Limit = 8
	hits, operationError = database.Search(operationContext, workspaceID, query)
	testutil.RequireNoError(test, operationError)
	seen = map[string]bool{}
	for _, hit := range hits {
		seen[hit.ID] = true
	}
	if !seen["deleted"] || !seen["old"] || seen["foreign"] || seen["other-space"] || seen["other-dimensions"] {
		test.Fatalf("archive or vector-space scope changed: %+v", hits)
	}
}

func TestNearestCandidateQueryCanUseItsModelDimensionIndex(test *testing.T) {
	database, workspaceID := isolatedRepository(test)
	operationContext := context.Background()
	model := "indexed-space"
	_, operationError := database.pool.Exec(operationContext, `INSERT INTO context_plugin.vectors(workspace_id,kind,key,version,chunk,model,content,embedding)
	 SELECT $1,'memory','memory-'||position,1,0,$2,'search document','[1,0]'::vector FROM generate_series(1,500) position`, workspaceID, model)
	testutil.RequireNoError(test, operationError)
	testutil.RequireNoError(test, database.ensureSearchIndex(operationContext, model, 2))
	transaction, operationError := database.pool.Begin(operationContext)
	testutil.RequireNoError(test, operationError)
	defer transaction.Rollback(operationContext)
	// Disable sequential scans only for this plan-usability assertion. The
	// runtime keeps normal costing and can choose an exact scan for small data.
	_, operationError = transaction.Exec(operationContext, `SET LOCAL enable_seqscan=off; SET LOCAL enable_sort=off; SET LOCAL plan_cache_mode=force_custom_plan; SET LOCAL hnsw.ef_search=64`)
	testutil.RequireNoError(test, operationError)
	rows, operationError := transaction.Query(operationContext, "EXPLAIN (COSTS OFF) "+memorySearchSQL(2, false), workspaceID, []string{"memory"}, false, false, []string{}, model, 2, "[1,0]", "", 5, 64, "%")
	testutil.RequireNoError(test, operationError)
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		testutil.RequireNoError(test, rows.Scan(&line))
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	testutil.RequireNoError(test, rows.Err())
	if !strings.Contains(plan.String(), "Index Scan using context_nearest_") {
		test.Fatalf("nearest retrieval cannot use its vector index:\n%s", plan.String())
	}
}
