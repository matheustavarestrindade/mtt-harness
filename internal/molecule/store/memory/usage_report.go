package memory

import (
	"context"
	"strings"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

func (usageStore *usage) Report(operationContext context.Context, query atom.UsageQuery) (atom.UsageReport, error) {
	if operationError := store.ValidateUsageQuery(query); operationError != nil {
		return atom.UsageReport{}, operationError
	}
	usageStore.store.mutex.RLock()
	defer usageStore.store.mutex.RUnlock()
	if operationError := operationContext.Err(); operationError != nil {
		return atom.UsageReport{}, operationError
	}
	var sessions map[atom.SessionID]bool
	if query.SessionID != "" {
		sessions = usageStore.store.sessionTree(query.SessionID)
	}
	builder := store.NewUsageReportBuilder(query)
	end := query.To.AddDate(0, 0, 1)
	for index, record := range usageStore.store.usage {
		if index%128 == 0 {
			if operationError := operationContext.Err(); operationError != nil {
				return atom.UsageReport{}, operationError
			}
		}
		if record.CreatedAt.Before(query.From) || !record.CreatedAt.Before(end) || query.InstanceID != "" && record.InstanceID != query.InstanceID || sessions != nil && !sessions[record.SessionID] || !strings.HasPrefix(record.Agent, query.AgentPrefix) {
			continue
		}
		group := atom.UsageBucket{Date: record.CreatedAt.UTC().Format(time.DateOnly), ModelID: record.ModelID, Statistics: atom.Statistics{Calls: 1, Input: record.Usage.Input, Output: record.Usage.Output, CacheRead: record.Usage.CacheRead, CacheWrite: record.Usage.CacheWrite, Reasoning: record.Usage.Reasoning}}
		if record.Usage.Cost != nil && record.Usage.Cost.Currency != "" {
			group.Statistics.Cost = record.Usage.Cost
			group.PricedCalls = 1
		}
		if record.Status == "error" {
			group.FailedCalls = 1
		}
		if operationError := builder.AddGroup(group); operationError != nil {
			return atom.UsageReport{}, operationError
		}
	}
	return builder.Report(), nil
}
