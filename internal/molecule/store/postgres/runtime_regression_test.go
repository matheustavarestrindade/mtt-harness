package postgres

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/eventbus"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/process"
	processmanager "github.com/matheustavarestrindade/mtt-harness/internal/organism/processes"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func regressionStore(test *testing.T) *Store {
	test.Helper()
	databaseURL := os.Getenv("MTT_TEST_DATABASE_URL")
	if databaseURL == "" {
		test.Skip("MTT_TEST_DATABASE_URL is not set")
	}
	parsed, operationError := url.Parse(databaseURL)
	testutil.RequireNoError(test, operationError)
	query := parsed.Query()
	query.Set("pool_max_conns", "1")
	parsed.RawQuery = query.Encode()
	database, operationError := Open(context.Background(), parsed.String())
	testutil.RequireNoError(test, operationError)
	test.Cleanup(func() {
		database.Close()
	})
	return database
}

func TestGlobalStatisticsQueueAndEventPersistence(test *testing.T) {
	database := regressionStore(test)
	operationContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	identifier := fmt.Sprintf("regression-%d", time.Now().UnixNano())
	session := atom.Session{ID: atom.SessionID(identifier), InstanceID: identifier, CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	_, operationError := database.Usage().All(operationContext)
	testutil.RequireNoError(test, operationError)
	testutil.RequireNoError(test, database.Usage().Save(operationContext, atom.UsageRecord{InstanceID: identifier, SessionID: session.ID, ModelID: "provider/model", Usage: atom.Usage{Input: 10, CacheRead: 5, Output: 3, Cost: &atom.Cost{Currency: "USD", Value: 0.25}}, CreatedAt: time.Now()}))
	statistics, operationError := database.Usage().All(operationContext)
	testutil.RequireNoError(test, operationError)
	if statistics.Calls < 1 || statistics.Input < 10 {
		test.Fatalf("missing global usage: %+v", statistics)
	}
	message := atom.Message{ID: identifier, SessionID: session.ID, Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "queued"}}, CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Queue().Enqueue(operationContext, message, 1))
	testutil.RequireNoError(test, database.Queue().Start(operationContext, message.ID))
	if operationError := database.Queue().Start(operationContext, message.ID); operationError == nil {
		test.Fatal("queue item was claimed twice")
	}
	messages, operationError := database.Sessions().Messages(operationContext, session.ID)
	testutil.RequireNoError(test, operationError)
	if len(messages) != 1 || messages[0].ID != message.ID {
		test.Fatalf("claim did not atomically append: %+v", messages)
	}
	entries, operationError := database.Queue().All(operationContext)
	testutil.RequireNoError(test, operationError)
	found := false
	for _, entry := range entries {
		if entry.Message.ID == message.ID {
			found = entry.Running
		}
	}
	if !found {
		test.Fatal("in-flight state did not survive a fresh store read")
	}
	testutil.RequireNoError(test, database.Queue().Finish(operationContext, message.ID))
	var previous uint64
	for index := 0; index < 2; index++ {
		runtime := harness.New()
		bus := eventbus.New(runtime)
		bus.SetRecorder(database.Events().Record)
		var delivered atom.Event
		bus.On("event", func(_ context.Context, event atom.Event) {
			delivered = event
		})
		testutil.RequireNoError(test, bus.Send(operationContext, atom.Event{Name: "event", InstanceID: identifier, SessionID: session.ID, Time: time.Now()}))
		stored, operationError := database.Events().Since(operationContext, identifier, previous)
		testutil.RequireNoError(test, operationError)
		if len(stored) != 1 || stored[0].Seq != delivered.Seq || delivered.Seq <= previous {
			test.Fatalf("stream/store sequences diverged: delivered=%+v stored=%+v", delivered, stored)
		}
		previous = delivered.Seq
	}
}

func TestProcessCompletionSurvivesOriginatingTurnCancellation(test *testing.T) {
	database := regressionStore(test)
	runtime := harness.New()
	bus := eventbus.New(runtime)
	bus.SetRecorder(database.Events().Record)
	manager := processmanager.New(process.New(2), database, runtime, bus, nil)
	defer func() {
		operationContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		testutil.RequireNoError(test, manager.Close(operationContext))
	}()
	identifier := fmt.Sprintf("notification-%d", time.Now().UnixNano())
	session := atom.Session{ID: atom.SessionID(identifier), InstanceID: identifier, CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Sessions().Save(context.Background(), session))
	operationContext, cancel := context.WithCancel(harness.WithSession(context.Background(), session))
	runningProcess, operationError := manager.Start(operationContext, atom.ProcessSpec{Command: "sh", Args: []string{"-c", "sleep 0.05; printf complete"}, Notify: atom.NotifyPolicy{Mode: atom.NotifyExit}})
	testutil.RequireNoError(test, operationError)
	cancel()
	if _, operationError := runningProcess.Wait(); operationError != nil {
		test.Fatal(operationError)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		record, operationError := database.Processes().Get(context.Background(), runningProcess.ID())
		testutil.RequireNoError(test, operationError)
		messages, operationError := database.Sessions().Messages(context.Background(), session.ID)
		testutil.RequireNoError(test, operationError)
		if record.Status == "stopped" && record.Exit != nil && len(messages) == 1 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	test.Fatal("exited process or notification was lost with the turn context")
}
