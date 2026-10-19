package loop

import (
	"context"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestReviewSourceOmitsEmptyStreamingPlaceholderAndLabelsPartialOutput(test *testing.T) {
	database := memory.New()
	instanceManager := instances.New(func(string) instances.SessionManager { return nil }, database.Settings())
	workspace := atom.InstanceSpec{ID: "source-workspace", Workspace: test.TempDir()}
	_, operationError := instanceManager.Start(context.Background(), workspace)
	testutil.RequireNoError(test, operationError)
	session := atom.Session{ID: "source-session", InstanceID: workspace.ID, CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Sessions().Save(context.Background(), session))
	testutil.RequireNoError(test, database.Sessions().Append(context.Background(), atom.Message{ID: "source-user", SessionID: session.ID, Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "task"}}, CreatedAt: time.Now()}))
	agentLoop := New(harness.New(), Config{Store: database, Instances: instanceManager})
	queue := NewQueue(agentLoop)
	defer queue.Close(context.Background())
	services := agentLoop.SessionWorkers(queue)
	agentLoop.mutex.Lock()
	agentLoop.activeRuns[session.ID] = &activeRun{session: session, cancel: func() {}, done: make(chan struct{})}
	agentLoop.mutex.Unlock()
	defer func() { agentLoop.mutex.Lock(); delete(agentLoop.activeRuns, session.ID); agentLoop.mutex.Unlock() }()
	agentLoop.beginLiveResponse(session, "live", "provider/model")
	source, operationError := services.Source(context.Background(), session.ID, 4096, false)
	testutil.RequireNoError(test, operationError)
	if source.LiveMessageID != "live" || len(source.Messages) != 1 || source.Messages[0].Role != atom.RoleUser {
		test.Fatalf("empty placeholder became a completed response: %+v", source.Messages)
	}
	agentLoop.appendLiveText(session.ID, "private display reasoning", true)
	source, operationError = services.Source(context.Background(), session.ID, 4096, false)
	testutil.RequireNoError(test, operationError)
	if len(source.Messages) != 1 {
		test.Fatal("disabled reasoning left an empty assistant placeholder")
	}
	agentLoop.appendLiveText(session.ID, "partial", false)
	source, operationError = services.Source(context.Background(), session.ID, 4096, false)
	testutil.RequireNoError(test, operationError)
	if len(source.Messages) != 2 || source.Messages[1].ID != source.LiveMessageID || source.Messages[1].Content[0].Text != "partial" {
		test.Fatal("partial response was not retained")
	}
}
