package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/api"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestWorkspaceUnlinkAPIKeepsFilesAndSavedData(test *testing.T) {
	operationContext := context.Background()
	database := memory.New()
	instanceManager := instances.New(func(string) instances.SessionManager { return nil }, database.Settings())
	workspace := test.TempDir()
	path := filepath.Join(workspace, "project.txt")
	testutil.RequireNoError(test, os.WriteFile(path, []byte("project remains"), 0600))
	instance, operationError := instanceManager.Start(operationContext, atom.InstanceSpec{Workspace: workspace, CreatedAt: time.Now()})
	testutil.RequireNoError(test, operationError)
	testutil.RequireNoError(test, database.Instances().Save(operationContext, instance.Spec()))
	session := atom.Session{ID: "unlink-api-session", InstanceID: instance.ID(), CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	testutil.RequireNoError(test, database.Sessions().Append(operationContext, atom.Message{ID: "saved-message", SessionID: session.ID, Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "keep history"}}, CreatedAt: time.Now()}))
	queue := loop.NewQueue(loop.New(harness.New(), loop.Config{Store: database, Instances: instanceManager}))
	defer queue.Close(operationContext)
	server := httptest.NewServer(api.New(api.Config{Token: "secret", Store: database, Instances: instanceManager, Queue: queue}).Handler())
	defer server.Close()
	url := server.URL + "/instances/" + instance.ID() + "/unlink"
	response := request(test, url, "wrong", http.MethodPost, nil)
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		test.Fatal("unlink did not require authentication")
	}
	testutil.RequireNoError(test, database.Queue().Enqueue(operationContext, atom.Message{ID: "queued-message", SessionID: session.ID, Role: atom.RoleUser, CreatedAt: time.Now()}, 128))
	response = request(test, url, "secret", http.MethodPost, nil)
	response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		test.Fatalf("busy unlink status: %d", response.StatusCode)
	}
	_, operationError = database.Queue().ClearPending(operationContext, session.ID)
	testutil.RequireNoError(test, operationError)
	before, operationError := database.Sessions().Messages(operationContext, session.ID)
	testutil.RequireNoError(test, operationError)
	response = request(test, url, "secret", http.MethodPost, nil)
	var result map[string]string
	testutil.RequireNoError(test, json.NewDecoder(response.Body).Decode(&result))
	response.Body.Close()
	if response.StatusCode != http.StatusOK || result["id"] != instance.ID() || result["status"] != "unlinked" {
		test.Fatalf("unlink response: %d %#v", response.StatusCode, result)
	}
	response = request(test, server.URL+"/instances", "secret", http.MethodGet, nil)
	var linked []atom.InstanceSpec
	testutil.RequireNoError(test, json.NewDecoder(response.Body).Decode(&linked))
	response.Body.Close()
	if len(linked) != 0 {
		test.Fatal("workspace remains linked")
	}
	for _, path := range []string{"/instances/" + instance.ID(), "/instances/" + instance.ID() + "/start", "/instances/" + instance.ID() + "/unlink"} {
		method := http.MethodGet
		if path != "/instances/"+instance.ID() {
			method = http.MethodPost
		}
		response = request(test, server.URL+path, "secret", method, nil)
		response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			test.Fatalf("unlinked route %s: %d", path, response.StatusCode)
		}
	}
	after, operationError := database.Sessions().Messages(operationContext, session.ID)
	testutil.RequireNoError(test, operationError)
	if len(after) != len(before) || after[0].Content[0].Text != "keep history" {
		test.Fatal("unlink deleted conversation data")
	}
	content, operationError := os.ReadFile(path)
	testutil.RequireNoError(test, operationError)
	if string(content) != "project remains" {
		test.Fatal("unlink changed filesystem content")
	}
}
