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
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestDeleteSessionRemovesOnlyIdleConversationData(test *testing.T) {
	operationContext := context.Background()
	database := memory.New()
	root := atom.Session{ID: "old", InstanceID: "workspace"}
	child := atom.Session{ID: "child", Parent: root.ID, InstanceID: root.InstanceID}
	for _, session := range []atom.Session{root, child} {
		testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	}
	testutil.RequireNoError(test, database.Sessions().Append(operationContext, atom.Message{ID: "private", SessionID: child.ID, Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "saved"}}}))
	runner := loop.New(harness.New(), loop.Config{Store: database})
	queue := loop.NewQueue(runner)
	defer queue.Close(operationContext)
	server := httptest.NewServer(api.New(api.Config{Token: "secret", Store: database, Queue: queue}).Handler())
	defer server.Close()
	response := request(test, server.URL+"/sessions/old", "wrong", http.MethodDelete, nil)
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		test.Fatalf("unauthenticated deletion: %d", response.StatusCode)
	}
	testutil.RequireNoError(test, database.Queue().Enqueue(operationContext, atom.Message{ID: "pending", SessionID: child.ID}, 128))
	response = request(test, server.URL+"/sessions/old", "secret", http.MethodDelete, nil)
	response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		test.Fatalf("queued child was deleted: %d", response.StatusCode)
	}
	_, operationError := database.Queue().ClearPending(operationContext, child.ID)
	testutil.RequireNoError(test, operationError)
	response = request(test, server.URL+"/sessions/old", "secret", http.MethodDelete, nil)
	var deletion struct {
		Status     string   `json:"status"`
		SessionIDs []string `json:"session_ids"`
	}
	testutil.RequireNoError(test, json.NewDecoder(response.Body).Decode(&deletion))
	response.Body.Close()
	if response.StatusCode != http.StatusOK || deletion.Status != "deleted" || len(deletion.SessionIDs) != 2 {
		test.Fatalf("deletion: %d %+v", response.StatusCode, deletion)
	}
	for _, identifier := range []string{"old", "child"} {
		response = request(test, server.URL+"/sessions/"+identifier, "secret", http.MethodGet, nil)
		response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			test.Fatalf("deleted session is still readable: %d", response.StatusCode)
		}
	}
	response = request(test, server.URL+"/sessions/old", "secret", http.MethodDelete, nil)
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		test.Fatalf("repeated deletion: %d", response.StatusCode)
	}
}
