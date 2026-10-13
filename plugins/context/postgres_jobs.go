package contextplugin

import (
	"encoding/json"
	"time"
)

// Claimers hold the existing workspace transaction lock. Only eligible jobs
// cross the database boundary; completed history is never decoded or sorted.
func (database *postgresTransaction) RunnableJobs(now time.Time, limit int) ([]memoryJob, error) {
	rows, operationError := database.transaction.Query(database.operationContext, `
 SELECT data FROM context_plugin.documents
 WHERE workspace_id=$1 AND kind='job' AND data->>'status' IN ('pending','running')
   AND (data->>'status'='pending' OR COALESCE((data->>'lease_until')::timestamptz,'-infinity')<$2)
   AND COALESCE((data->>'next_attempt')::timestamptz,'-infinity')<=$2
 ORDER BY COALESCE((data->>'priority')::int,0), (data->>'created_at')::timestamptz, key
 LIMIT $3`, database.workspaceID, now, limit)
	if operationError != nil {
		return nil, operationError
	}
	defer rows.Close()
	var jobs []memoryJob
	for rows.Next() {
		var encoded json.RawMessage
		if operationError := rows.Scan(&encoded); operationError != nil {
			return nil, operationError
		}
		var job memoryJob
		if operationError := json.Unmarshal(encoded, &job); operationError != nil {
			return nil, operationError
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (database *postgresTransaction) HasActiveJobs() (bool, error) {
	var active bool
	operationError := database.transaction.QueryRow(database.operationContext, `SELECT EXISTS(
 SELECT 1 FROM context_plugin.documents WHERE workspace_id=$1 AND kind='job' AND data->>'status' IN ('pending','running'))`, database.workspaceID).Scan(&active)
	return active, operationError
}
