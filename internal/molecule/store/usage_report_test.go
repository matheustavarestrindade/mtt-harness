package store_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/postgres"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestUsageReportMatchesRecordedUTCModelsAndScope(testingHandle *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		testingHandle.Run(backend, func(testingHandle *testing.T) {
			operationContext := context.Background()
			var database store.Store
			if backend == "memory" {
				database = memory.New()
			} else {
				databaseURL := os.Getenv("MTT_TEST_DATABASE_URL")
				if databaseURL == "" {
					testingHandle.Skip("MTT_TEST_DATABASE_URL not set")
				}
				opened, operationError := postgres.Open(operationContext, databaseURL)
				testutil.RequireNoError(testingHandle, operationError)
				database = opened
				testingHandle.Cleanup(func() { opened.Close() })
			}
			identifier := time.Now().UTC().Format("20060102150405.000000000") + backend
			instanceID := "usage-" + identifier
			foreignID := "foreign-" + identifier
			for _, id := range []string{instanceID, foreignID} {
				testutil.RequireNoError(testingHandle, database.Instances().Save(operationContext, atom.InstanceSpec{ID: id, Workspace: testingHandle.TempDir()}))
			}
			root := atom.Session{ID: atom.SessionID("root-" + identifier), InstanceID: instanceID}
			child := atom.Session{ID: atom.SessionID("child-" + identifier), Parent: root.ID, InstanceID: instanceID}
			foreign := atom.Session{ID: atom.SessionID("foreign-" + identifier), InstanceID: foreignID}
			for _, session := range []atom.Session{root, child, foreign} {
				testutil.RequireNoError(testingHandle, database.Sessions().Save(operationContext, session))
			}
			day := time.Date(2026, 2, 12, 0, 0, 0, 0, time.UTC)
			records := []atom.UsageRecord{
				{InstanceID: instanceID, SessionID: root.ID, ModelID: "synthetic-a/chat", CreatedAt: day, Usage: atom.Usage{Input: 100, Output: 20, CacheRead: 400, CacheWrite: 10, Reasoning: 7, Cost: &atom.Cost{Currency: "USD", Value: 0}}},
				{InstanceID: instanceID, SessionID: child.ID, ModelID: "synthetic-a/chat", CreatedAt: day.Add(23 * time.Hour), Usage: atom.Usage{Input: 50, Output: 10, Cost: &atom.Cost{Currency: "USD", Value: 0.25, Estimated: true}}},
				{InstanceID: instanceID, SessionID: root.ID, ModelID: "synthetic-a/chat", CreatedAt: day.Add(24 * time.Hour), Usage: atom.Usage{Input: 60, Output: 30, Cost: &atom.Cost{Currency: "EUR", Value: 0.5}}},
				// This local date is Feb 13, but the recorded UTC day is Feb 12.
				{InstanceID: instanceID, SessionID: root.ID, ModelID: "synthetic-b/plan", CreatedAt: time.Date(2026, 2, 13, 1, 0, 0, 0, time.FixedZone("plus-three", 3*3600)), Usage: atom.Usage{Input: 70, Output: 5, CacheRead: 700}},
				{InstanceID: instanceID, SourceSessionID: root.ID, Agent: "context.compactor", ModelID: "synthetic-b/worker", CreatedAt: day.Add(time.Hour), Usage: atom.Usage{Input: 30, Output: 40, Cost: &atom.Cost{Currency: "USD", Value: 0.1}}},
				{InstanceID: instanceID, SessionID: root.ID, ModelID: "synthetic-a/chat", CreatedAt: day.Add(2 * time.Hour), Status: "error"},
				{InstanceID: instanceID, SessionID: root.ID, ModelID: "synthetic-a/chat", CreatedAt: day.Add(-time.Nanosecond), Usage: atom.Usage{Input: 999}},
				{InstanceID: instanceID, SessionID: root.ID, ModelID: "synthetic-a/chat", CreatedAt: day.Add(48 * time.Hour), Usage: atom.Usage{Input: 999}},
				{InstanceID: foreignID, SessionID: foreign.ID, ModelID: "synthetic-a/chat", CreatedAt: day, Usage: atom.Usage{Input: 999}},
			}
			for _, record := range records {
				testutil.RequireNoError(testingHandle, database.Usage().Save(operationContext, record))
			}
			query := atom.UsageQuery{From: day, To: day.AddDate(0, 0, 1), InstanceID: instanceID}
			report, operationError := database.Usage().Report(operationContext, query)
			testutil.RequireNoError(testingHandle, operationError)
			if report.Timezone != "UTC" || report.From != "2026-02-12" || report.To != "2026-02-13" || len(report.Buckets) != 4 {
				testingHandle.Fatalf("date buckets: %+v", report)
			}
			if report.Statistics.Calls != 6 || report.Statistics.Input != 310 || report.Statistics.Output != 105 || report.Statistics.CacheRead != 1100 || report.Statistics.CacheWrite != 10 || report.Statistics.Reasoning != 7 || report.PricedCalls != 4 || report.FailedCalls != 1 {
				testingHandle.Fatalf("incorrect report: %+v", report)
			}
			if report.Statistics.Cost != nil || len(report.Statistics.Costs) != 2 || report.Statistics.Costs[0].Currency != "EUR" || report.Statistics.Costs[0].Value != .5 || report.Statistics.Costs[1].Value != .35 || !report.Statistics.Costs[1].Estimated {
				testingHandle.Fatalf("cost currencies/estimates: %+v", report.Statistics)
			}
			for _, bucket := range report.Buckets {
				if bucket.ModelID == "synthetic-b/plan" && (bucket.Date != "2026-02-12" || bucket.Statistics.Cost != nil || bucket.PricedCalls != 0) {
					testingHandle.Fatalf("unknown cost or timezone changed: %+v", bucket)
				}
			}
			query.InstanceID = ""
			query.SessionID = root.ID
			conversation, operationError := database.Usage().Report(operationContext, query)
			testutil.RequireNoError(testingHandle, operationError)
			if conversation.Statistics.Calls != 5 || conversation.Statistics.Input != 280 || conversation.PricedCalls != 3 {
				testingHandle.Fatalf("worker or foreign usage entered the conversation: %+v", conversation)
			}
			query.SessionID = ""
			query.InstanceID = instanceID
			query.AgentPrefix = "context."
			worker, operationError := database.Usage().Report(operationContext, query)
			testutil.RequireNoError(testingHandle, operationError)
			if worker.Statistics.Calls != 1 || worker.Statistics.Input != 30 || worker.Statistics.Cost.Value != .1 {
				testingHandle.Fatalf("worker prefix: %+v", worker)
			}
			query.AgentPrefix = "missing."
			empty, operationError := database.Usage().Report(operationContext, query)
			testutil.RequireNoError(testingHandle, operationError)
			if empty.Buckets == nil || len(empty.Buckets) != 0 || empty.Statistics.Calls != 0 || empty.Statistics.Cost != nil {
				testingHandle.Fatalf("empty report: %+v", empty)
			}
			cancelled, cancel := context.WithCancel(operationContext)
			cancel()
			if _, operationError = database.Usage().Report(cancelled, query); !errors.Is(operationError, context.Canceled) {
				testingHandle.Fatalf("cancellation: %v", operationError)
			}
		})
	}
}

func TestUsageReportLimitsAndExplicitZero(testingHandle *testing.T) {
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	query := atom.UsageQuery{From: day, To: day}
	for _, invalid := range []atom.UsageQuery{{From: day, To: day.Add(-time.Hour)}, {From: day, To: day.AddDate(0, 0, 366)}, {From: day, To: day, AgentPrefix: "watcher."}, {From: day, To: day, InstanceID: "a", SessionID: "b"}} {
		if store.ValidateUsageQuery(invalid) == nil {
			testingHandle.Fatalf("accepted invalid query: %+v", invalid)
		}
	}
	builder := store.NewUsageReportBuilder(query)
	testutil.RequireNoError(testingHandle, builder.AddGroup(atom.UsageBucket{Date: "2026-01-01", ModelID: "synthetic/zero", PricedCalls: 1, Statistics: atom.Statistics{Calls: 1, Cost: &atom.Cost{Currency: "USD", Value: 0}}}))
	if report := builder.Report(); report.Statistics.Cost == nil || report.Statistics.Cost.Value != 0 || report.PricedCalls != 1 {
		testingHandle.Fatalf("explicit zero became unknown: %+v", report)
	}
	for index := 1; index < store.MaxUsageGroups; index++ {
		testutil.RequireNoError(testingHandle, builder.AddGroup(atom.UsageBucket{Date: "2026-01-01", ModelID: time.Unix(int64(index), 0).String(), Statistics: atom.Statistics{Calls: 1}}))
	}
	if operationError := builder.AddGroup(atom.UsageBucket{Date: "2026-01-01", ModelID: "overflow"}); !errors.Is(operationError, store.ErrUsageReportLimit) {
		testingHandle.Fatalf("group bound: %v", operationError)
	}
}

func TestUsageReportCombinesCurrencyGroupsWithoutDuplicatingTokens(testingHandle *testing.T) {
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	builder := store.NewUsageReportBuilder(atom.UsageQuery{From: day, To: day})
	for _, currency := range []string{"USD", "EUR"} {
		testutil.RequireNoError(testingHandle, builder.AddGroup(atom.UsageBucket{Date: "2026-10-01", ModelID: "synthetic/model", Statistics: atom.Statistics{Calls: 1, Input: 10, Output: 5, Costs: []atom.Cost{{Currency: currency, Value: 0, Estimated: currency == "EUR"}}}, PricedCalls: 1}))
	}
	report := builder.Report()
	if len(report.Buckets) != 1 || report.Statistics.Calls != 2 || report.Statistics.Input != 20 || report.Statistics.Output != 10 || report.Statistics.Cost != nil || len(report.Statistics.Costs) != 2 {
		testingHandle.Fatalf("currency groups duplicated usage or collapsed currencies: %+v", report)
	}
	if !report.Statistics.Costs[0].Estimated || report.Statistics.Costs[0].Currency != "EUR" {
		testingHandle.Fatalf("cost estimation changed: %+v", report.Statistics.Costs)
	}
}
