package postgres

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type usage struct{ store *Store }

func (usageStore *usage) Save(operationContext context.Context, record atom.UsageRecord) error {
	var currency string
	var value float64
	var estimated bool
	if record.Usage.Cost != nil {
		currency = record.Usage.Cost.Currency
		value = record.Usage.Cost.Value
		estimated = record.Usage.Cost.Estimated
	}
	_, operationError := usageStore.store.pool.Exec(operationContext, `
		INSERT INTO usage_records (instance_id, session_id, model_id, input_tokens, cache_read_tokens,
			cache_write_tokens, output_tokens, reasoning_tokens, cost_currency, cost_value, created_at, cost_estimated,
			request_id, agent, run_id, source_session_id, duration_ms, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NULLIF($13,''), $14, $15, $16, $17, $18)
		ON CONFLICT (instance_id, request_id) WHERE request_id IS NOT NULL DO NOTHING`,
		record.InstanceID, string(record.SessionID), record.ModelID,
		record.Usage.Input, record.Usage.CacheRead, record.Usage.CacheWrite,
		record.Usage.Output, record.Usage.Reasoning, currency, value, record.CreatedAt, estimated,
		record.RequestID, record.Agent, record.RunID, string(record.SourceSessionID), record.Duration.Milliseconds(), record.Status)
	return operationError
}

func (usageStore *usage) Agents(operationContext context.Context, instanceID string) ([]atom.AgentStatistics, error) {
	rows, operationError := usageStore.store.pool.Query(operationContext, `SELECT agent, model_id, COUNT(*) FILTER (WHERE status='error'), COALESCE(SUM(duration_ms),0) FROM usage_records WHERE instance_id=$1 AND agent<>'' GROUP BY agent, model_id ORDER BY agent,model_id`, instanceID)
	if operationError != nil {
		return nil, operationError
	}
	var result []atom.AgentStatistics
	for rows.Next() {
		var item atom.AgentStatistics
		if operationError := rows.Scan(&item.Agent, &item.ModelID, &item.FailedCalls, &item.DurationMilliseconds); operationError != nil {
			rows.Close()
			return nil, operationError
		}
		result = append(result, item)
	}
	operationError = rows.Err()
	rows.Close()
	if operationError != nil {
		return nil, operationError
	}
	for index := range result {
		result[index].Statistics, operationError = usageStore.aggregate(operationContext, `SELECT * FROM usage_records WHERE instance_id=$1 AND agent=$2 AND model_id=$3`, instanceID, result[index].Agent, result[index].ModelID)
		if operationError != nil {
			return nil, operationError
		}
	}
	return result, nil
}

func (usageStore *usage) Session(operationContext context.Context, sessionID atom.SessionID) (atom.Statistics, error) {
	return usageStore.aggregate(operationContext, `
		WITH RECURSIVE tree AS (
			SELECT id FROM sessions WHERE id = $1
			UNION ALL
			SELECT s.id FROM sessions s JOIN tree t ON s.parent_id = t.id
		)
		SELECT * FROM usage_records WHERE session_id IN (SELECT id FROM tree)`, string(sessionID))
}

func (usageStore *usage) Instance(operationContext context.Context, identifier string) (atom.Statistics, error) {
	return usageStore.aggregate(operationContext, `SELECT * FROM usage_records WHERE instance_id = $1`, identifier)
}

func (usageStore *usage) All(operationContext context.Context) (atom.Statistics, error) {
	return usageStore.aggregate(operationContext, `SELECT * FROM usage_records`)
}

func (usageStore *usage) aggregate(operationContext context.Context, query string, arguments ...any) (atom.Statistics, error) {
	rows, operationError := usageStore.store.pool.Query(operationContext, `
		SELECT COUNT(*), COALESCE(SUM(input_tokens),0), COALESCE(SUM(cache_read_tokens),0),
			COALESCE(SUM(cache_write_tokens),0), COALESCE(SUM(output_tokens),0),
			COALESCE(SUM(reasoning_tokens),0)
		FROM (`+query+`) data`, arguments...)
	if operationError != nil {
		return atom.Statistics{}, operationError
	}
	defer rows.Close()
	var statistics atom.Statistics
	if rows.Next() {
		if operationError := rows.Scan(&statistics.Calls, &statistics.Input, &statistics.CacheRead, &statistics.CacheWrite, &statistics.Output, &statistics.Reasoning); operationError != nil {
			return atom.Statistics{}, operationError
		}
	}
	if operationError := rows.Err(); operationError != nil {
		return atom.Statistics{}, operationError
	}
	rows.Close()
	costRows, operationError := usageStore.store.pool.Query(operationContext, `
		SELECT cost_currency, COALESCE(SUM(cost_value),0), BOOL_OR(cost_estimated)
		FROM (`+query+`) data
		WHERE cost_currency <> ''
		GROUP BY cost_currency ORDER BY cost_currency`, arguments...)
	if operationError != nil {
		return atom.Statistics{}, operationError
	}
	defer costRows.Close()
	for costRows.Next() {
		var cost atom.Cost
		if operationError := costRows.Scan(&cost.Currency, &cost.Value, &cost.Estimated); operationError != nil {
			return atom.Statistics{}, operationError
		}
		statistics.Costs = append(statistics.Costs, cost)
	}
	if operationError := costRows.Err(); operationError != nil {
		return atom.Statistics{}, operationError
	}
	if len(statistics.Costs) == 1 {
		cost := statistics.Costs[0]
		statistics.Cost = &cost
	}
	return statistics, nil
}
