package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/api"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/eventbus"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/taskstate"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestTaskStateAPIAndRecordedProgressEvent(test *testing.T) {
	operationContext := context.Background()
	database := memory.New()
	session := atom.Session{ID: "tasks", InstanceID: "workspace"}
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	harnessRuntime := harness.New()
	bus := eventbus.New(harnessRuntime)
	runner := loop.New(harnessRuntime, loop.Config{Store: database, Bus: bus})
	server := httptest.NewServer(api.New(api.Config{Token: "secret", Store: database, Bus: bus}).Handler())
	defer server.Close()
	for _, scenario := range []struct {
		session, token string
		status         int
	}{{"tasks", "wrong", http.StatusUnauthorized}, {"missing", "secret", http.StatusNotFound}, {"tasks", "secret", http.StatusOK}} {
		response := request(test, server.URL+"/sessions/"+scenario.session+"/task-state", scenario.token, http.MethodGet, nil)
		response.Body.Close()
		if response.StatusCode != scenario.status {
			test.Fatalf("task state route: %d", response.StatusCode)
		}
	}
	update, operationError := taskstate.DecodeUpdate([]byte(`{"todo":[{"id":"1","title":"Implement","status":"in_progress"},{"id":"2","title":"Verify"}],"doing":{"title":"Implementing","description":"Current work."}}`))
	testutil.RequireNoError(test, operationError)
	_, operationError = runner.UpdateTaskState(operationContext, session, update)
	testutil.RequireNoError(test, operationError)
	response := request(test, server.URL+"/sessions/tasks/task-state", "secret", http.MethodGet, nil)
	var body map[string]json.RawMessage
	testutil.RequireNoError(test, json.NewDecoder(response.Body).Decode(&body))
	response.Body.Close()
	for _, name := range []string{"SessionID", "Todo", "Doing", "Revision", "UpdatedAt"} {
		if len(body[name]) == 0 {
			test.Fatalf("missing public field %s", name)
		}
	}
	if _, present := body["ResponsesSinceUpdate"]; present {
		test.Fatal("internal reminder counter was exposed")
	}
	var items []atom.TaskItem
	testutil.RequireNoError(test, json.Unmarshal(body["Todo"], &items))
	if len(items) != 2 || items[0].Status != atom.TaskInProgress {
		test.Fatalf("task state body: %s", body["Todo"])
	}
	events, operationError := database.Events().Since(operationContext, session.InstanceID, 0)
	testutil.RequireNoError(test, operationError)
	if len(events) != 1 || events[0].Name != atom.EventTaskStateUpdated || events[0].SessionID != session.ID {
		test.Fatalf("progress events: %+v", events)
	}
	_, operationError = database.Sessions().DeleteConversation(operationContext, session.ID)
	testutil.RequireNoError(test, operationError)
	response = request(test, server.URL+"/sessions/tasks/task-state", "secret", http.MethodGet, nil)
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		test.Fatal("deleted progress remained accessible")
	}
}
