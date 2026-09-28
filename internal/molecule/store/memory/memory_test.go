package memory

import (
	"context"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestUsageAggregatesAgents(test *testing.T) {
	database := New()
	operationContext := context.Background()
	parent := atom.Session{ID: "parent", InstanceID: "instance-1", CreatedAt: time.Now()}
	child := atom.Session{ID: "child", InstanceID: "instance-1", Parent: "parent", Depth: 1, CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, parent))
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, child))

	cost := &atom.Cost{Currency: "USD", Value: 0.5}
	records := []atom.UsageRecord{
		{InstanceID: "instance-1", SessionID: "parent", ModelID: "m", Usage: atom.Usage{Input: 100, Output: 10}},
		{InstanceID: "instance-1", SessionID: "child", ModelID: "m", Usage: atom.Usage{Input: 50, Output: 5, Cost: cost}},
	}
	for _, record := range records {
		testutil.RequireNoError(test, database.Usage().Save(operationContext, record))
	}
	parentStats, _ := database.Usage().Session(operationContext, "parent")
	if parentStats.Calls != 2 || parentStats.Input != 150 {
		test.Fatalf("parent statistics = %+v", parentStats)
	}
	childStats, _ := database.Usage().Session(operationContext, "child")
	if childStats.Calls != 1 || childStats.Input != 50 {
		test.Fatalf("child statistics = %+v", childStats)
	}
	instanceStats, _ := database.Usage().Instance(operationContext, "instance-1")
	if instanceStats.Calls != 2 || instanceStats.Cost == nil || instanceStats.Cost.Value != 0.5 {
		test.Fatalf("instance statistics = %+v", instanceStats)
	}
	all, _ := database.Usage().All(operationContext)
	if all.Calls != 2 {
		test.Fatalf("all statistics = %+v", all)
	}
}
