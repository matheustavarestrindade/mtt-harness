package memory

import (
	"context"
	"sort"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (database *Store) sessionTree(root atom.SessionID) map[atom.SessionID]bool {
	tree := map[atom.SessionID]bool{root: true}
	for changed := true; changed; {
		changed = false
		for sessionID, session := range database.sessions {
			if tree[session.Parent] && !tree[sessionID] {
				tree[sessionID] = true
				changed = true
			}
		}
	}
	return tree
}

func statistics(records []atom.UsageRecord) atom.Statistics {
	statistics := atom.Statistics{Calls: len(records)}
	totals := map[string]float64{}
	estimates := map[string]bool{}
	for _, record := range records {
		statistics.Input += record.Usage.Input
		statistics.CacheRead += record.Usage.CacheRead
		statistics.CacheWrite += record.Usage.CacheWrite
		statistics.Output += record.Usage.Output
		statistics.Reasoning += record.Usage.Reasoning
		if record.Usage.Cost != nil {
			totals[record.Usage.Cost.Currency] += record.Usage.Cost.Value
			estimates[record.Usage.Cost.Currency] = estimates[record.Usage.Cost.Currency] || record.Usage.Cost.Estimated
		}
	}
	currencies := make([]string, 0, len(totals))
	for currency := range totals {
		currencies = append(currencies, currency)
	}
	sort.Strings(currencies)
	for _, currency := range currencies {
		statistics.Costs = append(statistics.Costs, atom.Cost{Currency: currency, Value: totals[currency], Estimated: estimates[currency]})
	}
	if len(statistics.Costs) == 1 {
		cost := statistics.Costs[0]
		statistics.Cost = &cost
	}
	return statistics
}

type usage struct{ store *Store }

func (usageStore *usage) Save(operationContext context.Context, record atom.UsageRecord) error {
	usageStore.store.mutex.Lock()
	defer usageStore.store.mutex.Unlock()
	usageStore.store.usage = append(usageStore.store.usage, record)
	return nil
}

func (usageStore *usage) Session(operationContext context.Context, sessionID atom.SessionID) (atom.Statistics, error) {
	usageStore.store.mutex.RLock()
	defer usageStore.store.mutex.RUnlock()
	tree := usageStore.store.sessionTree(sessionID)
	var records []atom.UsageRecord
	for _, record := range usageStore.store.usage {
		if tree[record.SessionID] {
			records = append(records, record)
		}
	}
	return statistics(records), nil
}

func (usageStore *usage) Instance(operationContext context.Context, identifier string) (atom.Statistics, error) {
	usageStore.store.mutex.RLock()
	defer usageStore.store.mutex.RUnlock()
	var records []atom.UsageRecord
	for _, record := range usageStore.store.usage {
		if record.InstanceID == identifier {
			records = append(records, record)
		}
	}
	return statistics(records), nil
}

func (usageStore *usage) All(operationContext context.Context) (atom.Statistics, error) {
	usageStore.store.mutex.RLock()
	defer usageStore.store.mutex.RUnlock()
	return statistics(append([]atom.UsageRecord(nil), usageStore.store.usage...)), nil
}
