package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/postgres"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestAgentUsageStorageContract(test *testing.T) {
	stores := map[string]func(*testing.T) store.Store{"memory": func(*testing.T) store.Store { return memory.New() }, "postgres": func(test *testing.T) store.Store {
		url := os.Getenv("MTT_TEST_DATABASE_URL")
		if url == "" {
			test.Skip("Postgres integration disabled")
		}
		database, operationError := postgres.Open(context.Background(), url)
		testutil.RequireNoError(test, operationError)
		return database
	}}
	for name, open := range stores {
		test.Run(name, func(test *testing.T) {
			database := open(test)
			defer database.Close()
			operationContext := context.Background()
			workspace := "agent-usage-" + time.Now().Format("150405.000000000")
			record := atom.UsageRecord{InstanceID: workspace, Agent: "context.compactor", RunID: "job", RequestID: "attempt", SourceSessionID: "trace-only", ModelID: "fixture/worker", Usage: atom.Usage{Input: 10, Output: 4, Cost: &atom.Cost{Currency: "USD", Value: 0.01, Estimated: true}}, Status: "completed", Duration: 35 * time.Millisecond, CreatedAt: time.Now()}
			testutil.RequireNoError(test, database.Usage().Save(operationContext, record))
			testutil.RequireNoError(test, database.Usage().Save(operationContext, record))
			record.RequestID = "retry"
			record.Status = "error"
			testutil.RequireNoError(test, database.Usage().Save(operationContext, record))
			agents, operationError := database.Usage().Agents(operationContext, workspace)
			testutil.RequireNoError(test, operationError)
			if len(agents) != 1 || agents[0].Statistics.Calls != 2 || agents[0].FailedCalls != 1 || agents[0].DurationMilliseconds != 70 {
				test.Fatalf("wrong agent accounting: %+v", agents)
			}
			if agents[0].Statistics.Cost == nil || agents[0].Statistics.Cost.Value != 0.02 || !agents[0].Statistics.Cost.Estimated {
				test.Fatal("cost totals lost value or estimation state")
			}
			chat, operationError := database.Usage().Session(operationContext, "trace-only")
			testutil.RequireNoError(test, operationError)
			if chat.Calls != 0 {
				test.Fatal("trace session changed accounting ownership")
			}
		})
	}
}
