package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/postgres"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/taskstate"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func taskUpdate(test *testing.T, input string) atom.TaskStateUpdate {
	test.Helper()
	update, operationError := taskstate.DecodeUpdate([]byte(input))
	testutil.RequireNoError(test, operationError)
	return update
}

func taskStateDatabase(test *testing.T, backend string) store.Store {
	test.Helper()
	if backend == "memory" {
		return memory.New()
	}
	databaseURL := os.Getenv("MTT_TEST_DATABASE_URL")
	if databaseURL == "" {
		test.Skip("MTT_TEST_DATABASE_URL is not set")
	}
	database, operationError := postgres.Open(context.Background(), databaseURL)
	testutil.RequireNoError(test, operationError)
	return database
}

func TestTaskStateStorageContract(test *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		test.Run(backend, func(test *testing.T) {
			operationContext := context.Background()
			database := taskStateDatabase(test, backend)
			defer database.Close()
			identifier := fmt.Sprintf("tasks-%s-%d", backend, time.Now().UnixNano())
			session := atom.Session{ID: atom.SessionID(identifier), InstanceID: identifier, CreatedAt: time.Now()}
			child := atom.Session{ID: session.ID + "-child", InstanceID: identifier, Parent: session.ID, CreatedAt: time.Now()}
			for _, current := range []atom.Session{session, child} {
				testutil.RequireNoError(test, database.Sessions().Save(operationContext, current))
			}
			state, operationError := database.TaskStates().Get(operationContext, session.ID)
			testutil.RequireNoError(test, operationError)
			if taskstate.Active(state) || state.Revision != 0 || state.Todo == nil {
				test.Fatalf("initial state: %+v", state)
			}
			state, operationError = database.TaskStates().Update(operationContext, session.ID, taskUpdate(test, `{"todo":[{"id":"1","title":"Inspect","status":"in_progress"},{"id":"2","title":"Verify"}],"doing":{"title":"Inspecting","description":"Reading the relevant path."}}`))
			testutil.RequireNoError(test, operationError)
			if state.Revision != 1 || state.Todo[1].Status != atom.TaskPending || state.Doing == nil {
				test.Fatalf("created state: %+v", state)
			}
			state.Todo[0].Title = "caller mutation"
			state.Doing.Title = "caller mutation"
			state, operationError = database.TaskStates().Get(operationContext, session.ID)
			testutil.RequireNoError(test, operationError)
			if state.Todo[0].Title != "Inspect" || state.Doing.Title != "Inspecting" {
				test.Fatal("read alias changed persisted state")
			}
			childState, operationError := database.TaskStates().Get(operationContext, child.ID)
			testutil.RequireNoError(test, operationError)
			if taskstate.Active(childState) {
				test.Fatal("child inherited parent progress")
			}
			for range 7 {
				testutil.RequireNoError(test, database.TaskStates().RecordResponse(operationContext, session.ID, state.Revision))
			}
			state, operationError = database.TaskStates().Get(operationContext, session.ID)
			testutil.RequireNoError(test, operationError)
			if state.ResponsesSinceUpdate != 3 {
				test.Fatalf("response counter: %d", state.ResponsesSinceUpdate)
			}
			previousRevision := state.Revision
			state, operationError = database.TaskStates().Update(operationContext, session.ID, taskUpdate(test, `{"doing":{"title":"Checking","description":"Checking the existing tests."}}`))
			testutil.RequireNoError(test, operationError)
			testutil.RequireNoError(test, database.TaskStates().RecordResponse(operationContext, session.ID, previousRevision))
			state, operationError = database.TaskStates().Get(operationContext, session.ID)
			testutil.RequireNoError(test, operationError)
			if state.ResponsesSinceUpdate != 0 || len(state.Todo) != 2 {
				test.Fatal("response from before the update aged the new state")
			}
			before, operationError := json.Marshal(state)
			testutil.RequireNoError(test, operationError)
			_, operationError = database.TaskStates().Update(operationContext, session.ID, taskUpdate(test, `{"todo":[{"id":"1","status":"done"},{"id":"unknown","status":"in_progress"}]}`))
			if operationError == nil {
				test.Fatal("new task without title was accepted")
			}
			unchanged, operationError := database.TaskStates().Get(operationContext, session.ID)
			testutil.RequireNoError(test, operationError)
			after, operationError := json.Marshal(unchanged)
			testutil.RequireNoError(test, operationError)
			if string(before) != string(after) {
				test.Fatal("failed update changed state")
			}
			state, operationError = database.TaskStates().Update(operationContext, session.ID, taskUpdate(test, `{"todo":[{"id":"1","status":"done"},{"id":"2","status":"in_progress"}]}`))
			testutil.RequireNoError(test, operationError)
			if len(state.Todo) != 2 || state.Todo[0].Status != atom.TaskDone || state.Doing.Title != "Checking" {
				test.Fatal("partial progress lost completed items or doing")
			}
			state, changed, operationError := database.TaskStates().Pause(operationContext, session.ID)
			testutil.RequireNoError(test, operationError)
			if !changed || state.Doing != nil || len(state.Todo) != 2 || state.Todo[0].Status != atom.TaskDone {
				test.Fatal("pause lost TODO or kept DOING")
			}
			state, operationError = database.TaskStates().Update(operationContext, session.ID, taskUpdate(test, `{"todo":[{"id":"2","status":"done"}]}`))
			testutil.RequireNoError(test, operationError)
			if taskstate.Active(state) || state.Todo == nil || state.ResponsesSinceUpdate != 0 {
				test.Fatal("finished work remained visible")
			}
			_, operationError = database.TaskStates().Update(operationContext, session.ID, taskUpdate(test, `{"doing":{"title":"Another task","description":"Working without a checklist."}}`))
			testutil.RequireNoError(test, operationError)
			state, operationError = database.TaskStates().Update(operationContext, session.ID, taskUpdate(test, `{"todo":[],"doing":null}`))
			testutil.RequireNoError(test, operationError)
			if taskstate.Active(state) {
				test.Fatal("explicit clear retained task state")
			}
			for _, current := range []atom.Session{session, child} {
				_, operationError = database.TaskStates().Update(operationContext, current.ID, taskUpdate(test, `{"todo":[{"id":"unfinished","title":"Unfinished work"}],"doing":{"title":"Working","description":"Work in progress."}}`))
				testutil.RequireNoError(test, operationError)
			}
			anchor := atom.Message{ID: identifier + "-anchor", SessionID: session.ID, Role: atom.RoleUser, CreatedAt: time.Now()}
			testutil.RequireNoError(test, database.Sessions().Append(operationContext, anchor))
			_, operationError = database.Sessions().DeleteAfter(operationContext, session.ID, anchor.ID)
			testutil.RequireNoError(test, operationError)
			state, operationError = database.TaskStates().Get(operationContext, session.ID)
			testutil.RequireNoError(test, operationError)
			if taskstate.Active(state) {
				test.Fatal("revert retained task state from discarded work")
			}
			_, operationError = database.Sessions().DeleteConversation(operationContext, session.ID)
			testutil.RequireNoError(test, operationError)
			for _, deleted := range []atom.SessionID{session.ID, child.ID} {
				_, operationError = database.TaskStates().Update(operationContext, deleted, taskUpdate(test, `{"doing":{"title":"late","description":"late write"}}`))
				if !errors.Is(operationError, store.ErrSessionDeleted) {
					test.Fatalf("deleted state write: %v", operationError)
				}
				if operationError := database.TaskStates().RecordResponse(operationContext, deleted, 1); !errors.Is(operationError, store.ErrSessionDeleted) {
					test.Fatalf("deleted counter write: %v", operationError)
				}
				_, operationError = database.TaskStates().Get(operationContext, deleted)
				if !errors.Is(operationError, store.ErrSessionDeleted) {
					test.Fatal("deleted task state remained public")
				}
			}
		})
	}
}

func TestTaskStateConcurrentPartialUpdates(test *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		test.Run(backend, func(test *testing.T) {
			database := taskStateDatabase(test, backend)
			defer database.Close()
			operationContext := context.Background()
			session := atom.Session{ID: atom.SessionID(fmt.Sprintf("concurrent-tasks-%s-%d", backend, time.Now().UnixNano())), InstanceID: "tasks", CreatedAt: time.Now()}
			testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
			var workers sync.WaitGroup
			failures := make(chan error, 12)
			for index := range 12 {
				update := taskUpdate(test, fmt.Sprintf(`{"todo":[{"id":"%d","title":"Task %d"}]}`, index, index))
				workers.Add(1)
				go func() {
					defer workers.Done()
					_, operationError := database.TaskStates().Update(operationContext, session.ID, update)
					failures <- operationError
				}()
			}
			workers.Wait()
			close(failures)
			for operationError := range failures {
				testutil.RequireNoError(test, operationError)
			}
			state, operationError := database.TaskStates().Get(operationContext, session.ID)
			testutil.RequireNoError(test, operationError)
			if len(state.Todo) != 12 || state.Revision != 12 {
				test.Fatalf("lost updates: %+v", state)
			}
		})
	}
}

func TestTaskStatePersistsAcrossPostgresConnections(test *testing.T) {
	database := taskStateDatabase(test, "postgres")
	operationContext := context.Background()
	session := atom.Session{ID: atom.SessionID(fmt.Sprintf("persistent-tasks-%d", time.Now().UnixNano())), InstanceID: "tasks", CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	state, operationError := database.TaskStates().Update(operationContext, session.ID, taskUpdate(test, `{"todo":[{"id":"1","title":"Survive restart"}],"doing":{"title":"Persisting","description":"Keep the saved activity."}}`))
	testutil.RequireNoError(test, operationError)
	for range 3 {
		testutil.RequireNoError(test, database.TaskStates().RecordResponse(operationContext, session.ID, state.Revision))
	}
	testutil.RequireNoError(test, database.Close())
	database = taskStateDatabase(test, "postgres")
	defer database.Close()
	restored, operationError := database.TaskStates().Get(operationContext, session.ID)
	testutil.RequireNoError(test, operationError)
	if restored.Revision != state.Revision || restored.ResponsesSinceUpdate != 3 || restored.Doing.Description != state.Doing.Description || len(restored.Todo) != 1 {
		test.Fatalf("restored state: %+v", restored)
	}
}

func TestPostgresTaskStateRowsAreLazyAndPurged(test *testing.T) {
	database := taskStateDatabase(test, "postgres")
	defer database.Close()
	operationContext := context.Background()
	connection, operationError := pgx.Connect(operationContext, os.Getenv("MTT_TEST_DATABASE_URL"))
	testutil.RequireNoError(test, operationError)
	defer connection.Close(operationContext)
	session := atom.Session{ID: atom.SessionID(fmt.Sprintf("task-row-%d", time.Now().UnixNano())), InstanceID: "task-row-contract", CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	_, operationError = database.TaskStates().Get(operationContext, session.ID)
	testutil.RequireNoError(test, operationError)
	var rows int
	testutil.RequireNoError(test, connection.QueryRow(operationContext, `SELECT count(*) FROM session_task_state WHERE session_id=$1`, string(session.ID)).Scan(&rows))
	if rows != 0 {
		test.Fatal("reading empty progress created a database record")
	}
	_, operationError = database.TaskStates().Update(operationContext, session.ID, taskUpdate(test, `{"doing":{"title":"Private work","description":"Saved task details."}}`))
	testutil.RequireNoError(test, operationError)
	testutil.RequireNoError(test, connection.QueryRow(operationContext, `SELECT count(*) FROM session_task_state WHERE session_id=$1 AND doing->>'Title'='Private work'`, string(session.ID)).Scan(&rows))
	if rows != 1 {
		test.Fatal("task state was not in the actual database row")
	}
	_, operationError = database.Sessions().DeleteConversation(operationContext, session.ID)
	testutil.RequireNoError(test, operationError)
	testutil.RequireNoError(test, connection.QueryRow(operationContext, `SELECT count(*) FROM session_task_state WHERE session_id=$1`, string(session.ID)).Scan(&rows))
	if rows != 0 {
		test.Fatal("private task data remained after conversation deletion")
	}
}
