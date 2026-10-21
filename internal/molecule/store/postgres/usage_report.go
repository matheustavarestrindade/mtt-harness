package postgres

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

func (usageStore *usage) Report(operationContext context.Context, query atom.UsageQuery) (atom.UsageReport, error) {
	if operationError := store.ValidateUsageQuery(query); operationError != nil {
		return atom.UsageReport{}, operationError
	}
	// Group at the database boundary. Date, model, currency, and totals all come
	// from this one statement snapshot; no request bodies or secrets are read.
	rows, operationError := usageStore.store.pool.Query(operationContext, `
		WITH RECURSIVE conversation AS (
			SELECT id FROM sessions WHERE id=$4 AND $4<>''
			UNION ALL SELECT child.id FROM sessions child JOIN conversation parent ON child.parent_id=parent.id
		)
		SELECT to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD'), model_id, cost_currency,
			COUNT(*),COALESCE(SUM(input_tokens),0),COALESCE(SUM(output_tokens),0),
			COALESCE(SUM(cache_read_tokens),0),COALESCE(SUM(cache_write_tokens),0),
			COALESCE(SUM(reasoning_tokens),0),COUNT(*) FILTER(WHERE status='error'),
			COALESCE(SUM(cost_value),0),BOOL_OR(cost_estimated)
		FROM usage_records
		WHERE created_at >= $1 AND created_at < $2
			AND ($3='' OR instance_id=$3)
			AND ($4='' OR session_id IN (SELECT id FROM conversation))
			AND ($5='' OR left(agent,length($5))=$5)
		GROUP BY 1,model_id,cost_currency
		ORDER BY 1,model_id,cost_currency LIMIT $6`, query.From, query.To.AddDate(0, 0, 1), query.InstanceID, string(query.SessionID), query.AgentPrefix, store.MaxUsageGroups+1)
	if operationError != nil {
		return atom.UsageReport{}, operationError
	}
	defer rows.Close()
	builder := store.NewUsageReportBuilder(query)
	for rows.Next() {
		var group atom.UsageBucket
		var cost atom.Cost
		if operationError = rows.Scan(&group.Date, &group.ModelID, &cost.Currency, &group.Statistics.Calls, &group.Statistics.Input, &group.Statistics.Output, &group.Statistics.CacheRead, &group.Statistics.CacheWrite, &group.Statistics.Reasoning, &group.FailedCalls, &cost.Value, &cost.Estimated); operationError != nil {
			return atom.UsageReport{}, operationError
		}
		if cost.Currency != "" {
			group.Statistics.Cost = &cost
			group.PricedCalls = group.Statistics.Calls
		}
		if operationError = builder.AddGroup(group); operationError != nil {
			return atom.UsageReport{}, operationError
		}
	}
	if operationError = rows.Err(); operationError != nil {
		return atom.UsageReport{}, operationError
	}
	return builder.Report(), nil
}
