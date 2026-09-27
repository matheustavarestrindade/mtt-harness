package memory

import (
	"context"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func TestUsageAggregatesAgents(t *testing.T) {
	database := New()
	ctx := context.Background()
	parent := atom.Session{ID: "parent", InstanceID: "instance-1", CreatedAt: time.Now()}
	child := atom.Session{ID: "child", InstanceID: "instance-1", Parent: "parent", Depth: 1, CreatedAt: time.Now()}
	if err := database.Sessions().Save(ctx, parent); err != nil {
		t.Fatal(err)
	}
	if err := database.Sessions().Save(ctx, child); err != nil {
		t.Fatal(err)
	}
	cost := &atom.Cost{Currency: "USD", Value: 0.5}
	records := []atom.UsageRecord{
		{InstanceID: "instance-1", SessionID: "parent", ModelID: "m", Usage: atom.Usage{Input: 100, Output: 10}},
		{InstanceID: "instance-1", SessionID: "child", ModelID: "m", Usage: atom.Usage{Input: 50, Output: 5, Cost: cost}},
	}
	for _, record := range records {
		if err := database.Usage().Save(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	parentStats, _ := database.Usage().Session(ctx, "parent")
	if parentStats.Calls != 2 || parentStats.Input != 150 {
		t.Fatalf("parent statistics = %+v", parentStats)
	}
	childStats, _ := database.Usage().Session(ctx, "child")
	if childStats.Calls != 1 || childStats.Input != 50 {
		t.Fatalf("child statistics = %+v", childStats)
	}
	instanceStats, _ := database.Usage().Instance(ctx, "instance-1")
	if instanceStats.Calls != 2 || instanceStats.Cost == nil || instanceStats.Cost.Value != 0.5 {
		t.Fatalf("instance statistics = %+v", instanceStats)
	}
	all, _ := database.Usage().All(ctx)
	if all.Calls != 2 {
		t.Fatalf("all statistics = %+v", all)
	}
}
