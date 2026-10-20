package store

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

const MaxUsageDays = 366
const MaxUsageGroups = 10000

var ErrUsageReportLimit = errors.New("usage report exceeds 10000 day/model/currency groups")

// ValidateUsageQuery checks bounds before any storage query or allocation.
func ValidateUsageQuery(query atom.UsageQuery) error {
	for _, date := range []time.Time{query.From, query.To} {
		utc := date.UTC()
		if date.IsZero() || utc.Year() < 1 || utc.Year() > 9999 || utc.Hour() != 0 || utc.Minute() != 0 || utc.Second() != 0 || utc.Nanosecond() != 0 {
			return fmt.Errorf("usage dates must be UTC calendar dates")
		}
	}
	if query.To.Before(query.From) {
		return fmt.Errorf("usage from date is after to date")
	}
	if query.To.Sub(query.From) > time.Duration(MaxUsageDays-1)*24*time.Hour {
		return fmt.Errorf("usage date range exceeds %d days", MaxUsageDays)
	}
	if query.InstanceID != "" && query.SessionID != "" {
		return fmt.Errorf("usage scope accepts instance_id or session_id, not both")
	}
	if query.AgentPrefix != "" && query.InstanceID == "" {
		return fmt.Errorf("agent_prefix requires instance_id")
	}
	for _, value := range []string{query.InstanceID, string(query.SessionID), query.AgentPrefix} {
		if len(value) > 256 || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return fmt.Errorf("usage scope value is invalid")
		}
	}
	return nil
}

type usageBucketKey struct{ date, model string }
type usageGroupKey struct{ date, model, currency string }

// UsageReportBuilder shares aggregation semantics between storage adapters.
// Adapters supply disjoint groups or individual records from one read snapshot.
type UsageReportBuilder struct {
	report  atom.UsageReport
	buckets map[usageBucketKey]*atom.UsageBucket
	groups  map[usageGroupKey]struct{}
}

func NewUsageReportBuilder(query atom.UsageQuery) *UsageReportBuilder {
	return &UsageReportBuilder{report: atom.UsageReport{From: query.From.UTC().Format(time.DateOnly), To: query.To.UTC().Format(time.DateOnly), Timezone: "UTC", Buckets: []atom.UsageBucket{}}, buckets: map[usageBucketKey]*atom.UsageBucket{}, groups: map[usageGroupKey]struct{}{}}
}

func (builder *UsageReportBuilder) AddGroup(group atom.UsageBucket) error {
	currency := ""
	if group.Statistics.Cost != nil {
		currency = group.Statistics.Cost.Currency
	}
	key := usageGroupKey{group.Date, group.ModelID, currency}
	if _, exists := builder.groups[key]; !exists {
		if len(builder.groups) >= MaxUsageGroups {
			return ErrUsageReportLimit
		}
		builder.groups[key] = struct{}{}
	}
	bucketKey := usageBucketKey{group.Date, group.ModelID}
	bucket := builder.buckets[bucketKey]
	if bucket == nil {
		bucket = &atom.UsageBucket{Date: group.Date, ModelID: group.ModelID}
		builder.buckets[bucketKey] = bucket
	}
	if operationError := addUsageStatistics(&bucket.Statistics, group.Statistics); operationError != nil {
		return operationError
	}
	if operationError := addUsageStatistics(&builder.report.Statistics, group.Statistics); operationError != nil {
		return operationError
	}
	bucket.PricedCalls += group.PricedCalls
	bucket.FailedCalls += group.FailedCalls
	builder.report.PricedCalls += group.PricedCalls
	builder.report.FailedCalls += group.FailedCalls
	return nil
}

func (builder *UsageReportBuilder) Report() atom.UsageReport {
	result := builder.report
	result.Buckets = make([]atom.UsageBucket, 0, len(builder.buckets))
	for _, bucket := range builder.buckets {
		result.Buckets = append(result.Buckets, *bucket)
	}
	sort.Slice(result.Buckets, func(first, second int) bool {
		if result.Buckets[first].Date == result.Buckets[second].Date {
			return result.Buckets[first].ModelID < result.Buckets[second].ModelID
		}
		return result.Buckets[first].Date < result.Buckets[second].Date
	})
	return result
}

func addUsageStatistics(target *atom.Statistics, source atom.Statistics) error {
	target.Calls += source.Calls
	target.Input += source.Input
	target.Output += source.Output
	target.CacheRead += source.CacheRead
	target.CacheWrite += source.CacheWrite
	target.Reasoning += source.Reasoning
	costs := source.Costs
	if len(costs) == 0 && source.Cost != nil {
		costs = []atom.Cost{*source.Cost}
	}
	for _, cost := range costs {
		if cost.Currency == "" || math.IsNaN(cost.Value) || math.IsInf(cost.Value, 0) {
			return fmt.Errorf("usage contains an invalid recorded cost")
		}
		position := sort.Search(len(target.Costs), func(index int) bool { return target.Costs[index].Currency >= cost.Currency })
		if position == len(target.Costs) || target.Costs[position].Currency != cost.Currency {
			target.Costs = append(target.Costs, atom.Cost{})
			copy(target.Costs[position+1:], target.Costs[position:])
			target.Costs[position] = cost
		} else {
			target.Costs[position].Value += cost.Value
			target.Costs[position].Estimated = target.Costs[position].Estimated || cost.Estimated
		}
	}
	target.Cost = nil
	if len(target.Costs) == 1 {
		cost := target.Costs[0]
		target.Cost = &cost
	}
	return nil
}
