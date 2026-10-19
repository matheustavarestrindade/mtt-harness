package store_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/postgres"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestExecutionProfilePersistsPrivatelyAndCannotBeWidenedByLifecycleSave(test *testing.T) {
	for _, kind := range []string{"memory", "postgres"} {
		test.Run(kind, func(test *testing.T) {
			var database store.Store = memory.New()
			if kind == "postgres" {
				url := os.Getenv("MTT_TEST_DATABASE_URL")
				if url == "" {
					test.Skip("MTT_TEST_DATABASE_URL is not set")
				}
				opened, operationError := postgres.Open(context.Background(), url)
				testutil.RequireNoError(test, operationError)
				database = opened
			}
			defer database.Close()
			identifier := time.Now().Format("150405.000000000")
			workspace := atom.InstanceSpec{ID: "profile-" + identifier}
			testutil.RequireNoError(test, database.Instances().Save(context.Background(), workspace))
			session := atom.Session{ID: atom.SessionID("worker-" + identifier), InstanceID: workspace.ID, Model: "synthetic/model", CreatedAt: time.Now(), Execution: &atom.ExecutionProfile{Kind: "watcher", RunID: "run", Agent: "watcher.test", SystemPrompt: "private prompt", Tools: []atom.ToolGrant{{Name: "file_actions", Operations: []string{"read"}}}}}
			testutil.RequireNoError(test, database.Sessions().Save(context.Background(), session))
			session.Execution.Tools[0].Operations[0] = "write"
			testutil.RequireNoError(test, database.Sessions().Save(context.Background(), session))
			saved, operationError := database.Sessions().Get(context.Background(), session.ID)
			testutil.RequireNoError(test, operationError)
			if !saved.IsWatcher() || saved.Execution.Tools[0].Operations[0] != "read" {
				test.Fatal("stored authorization changed")
			}
			encoded, operationError := json.Marshal(saved)
			testutil.RequireNoError(test, operationError)
			var public map[string]any
			testutil.RequireNoError(test, json.Unmarshal(encoded, &public))
			if public["Kind"] != "watcher" || public["Execution"] != nil {
				test.Fatalf("profile visibility: %s", encoded)
			}
			_, operationError = database.Sessions().DeleteConversation(context.Background(), session.ID)
			testutil.RequireNoError(test, operationError)
		})
	}
}
