package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/api"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestWorkerSessionsStayOutOfConversationListsAndKeepProfilesPrivate(test *testing.T) {
	database := memory.New()
	operationContext := context.Background()
	workspace := atom.InstanceSpec{ID: "workspace", Workspace: test.TempDir()}
	testutil.RequireNoError(test, database.Instances().Save(operationContext, workspace))
	for _, session := range []atom.Session{{ID: "main", InstanceID: workspace.ID, CreatedAt: time.Now()}, {ID: "agent", InstanceID: workspace.ID, Parent: "main", CreatedAt: time.Now()}, {ID: "watcher", InstanceID: workspace.ID, Parent: "main", CreatedAt: time.Now(), Execution: &atom.ExecutionProfile{Kind: "watcher", SystemPrompt: "private-profile-text", Tools: []atom.ToolGrant{{Name: "private-tool"}}}}} {
		testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	}
	server := httptest.NewServer(api.New(api.Config{Store: database, Token: "secret"}).Handler())
	defer server.Close()
	get := func(path string, output any) {
		test.Helper()
		request, operationError := http.NewRequest(http.MethodGet, server.URL+path, nil)
		testutil.RequireNoError(test, operationError)
		request.Header.Set("Authorization", "Bearer secret")
		response, operationError := http.DefaultClient.Do(request)
		testutil.RequireNoError(test, operationError)
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			test.Fatalf("%s: %d", path, response.StatusCode)
		}
		testutil.RequireNoError(test, json.NewDecoder(response.Body).Decode(output))
	}
	var sessions []atom.Session
	get("/instances/workspace/sessions", &sessions)
	if len(sessions) != 2 {
		test.Fatalf("worker appeared in session list: %+v", sessions)
	}
	var agents []atom.SessionID
	get("/sessions/main/agents", &agents)
	if len(agents) != 1 || agents[0] != "agent" {
		test.Fatalf("worker appeared in agent tree: %v", agents)
	}
	var worker map[string]any
	get("/sessions/watcher", &worker)
	data, _ := json.Marshal(worker)
	if worker["Kind"] != "watcher" || strings.Contains(string(data), "private-profile") || strings.Contains(string(data), "private-tool") || worker["Execution"] != nil {
		test.Fatalf("profile disclosure: %s", data)
	}
}
