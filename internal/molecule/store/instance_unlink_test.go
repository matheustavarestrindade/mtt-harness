package store_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/postgres"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestInstanceUnlinkPreservesDataAndRejectsStaleWriters(test *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		test.Run(backend, func(test *testing.T) {
			operationContext := context.Background()
			var database store.Store = memory.New()
			if backend == "postgres" {
				url := os.Getenv("MTT_TEST_DATABASE_URL")
				if url == "" {
					test.Skip("MTT_TEST_DATABASE_URL is not set")
				}
				var operationError error
				database, operationError = postgres.Open(operationContext, url)
				testutil.RequireNoError(test, operationError)
			}
			test.Cleanup(func() { testutil.RequireNoError(test, database.Close()) })
			identifier := fmt.Sprintf("unlink-%s-%d", backend, time.Now().UnixNano())
			workspace := test.TempDir()
			path := filepath.Join(workspace, "keep.txt")
			testutil.RequireNoError(test, os.WriteFile(path, []byte("workspace content stays\n"), 0600))
			instance := atom.InstanceSpec{ID: identifier, Workspace: workspace, CreatedAt: time.Now()}
			testutil.RequireNoError(test, database.Instances().Save(operationContext, instance))
			session := atom.Session{ID: atom.SessionID(identifier + "-chat"), InstanceID: identifier, CreatedAt: time.Now()}
			testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
			message := atom.Message{ID: identifier + "-message", SessionID: session.ID, Role: atom.RoleUser, CreatedAt: time.Now(), Content: []atom.Content{{Type: atom.Text, Text: "saved conversation"}}}
			testutil.RequireNoError(test, database.Sessions().Append(operationContext, message))
			testutil.RequireNoError(test, database.Settings().Save(operationContext, identifier, "plugin.context", `{"enabled":true}`))
			testutil.RequireNoError(test, database.Secrets().SaveInstanceKey(operationContext, identifier, "synthetic-provider", "saved-test-key"))
			testutil.RequireNoError(test, database.Usage().Save(operationContext, atom.UsageRecord{InstanceID: identifier, SessionID: session.ID, RequestID: identifier + "-request", ModelID: "synthetic-model", Usage: atom.Usage{Input: 41, Output: 9}, CreatedAt: time.Now()}))
			beforeMessages, operationError := database.Sessions().Messages(operationContext, session.ID)
			testutil.RequireNoError(test, operationError)
			beforeUsage, operationError := database.Usage().Instance(operationContext, identifier)
			testutil.RequireNoError(test, operationError)
			queued := atom.Message{ID: identifier + "-queued", SessionID: session.ID, Role: atom.RoleUser, CreatedAt: time.Now()}
			testutil.RequireNoError(test, database.Queue().Enqueue(operationContext, queued, 128))
			if operationError := database.Instances().Unlink(operationContext, identifier); !errors.Is(operationError, store.ErrInstanceBusy) {
				test.Fatalf("pending work was not protected: %v", operationError)
			}
			_, operationError = database.Queue().ClearPending(operationContext, session.ID)
			testutil.RequireNoError(test, operationError)
			process := atom.ProcessRecord{ID: identifier + "-process", InstanceID: identifier, SessionID: session.ID, Status: "running", StartedAt: time.Now()}
			testutil.RequireNoError(test, database.Processes().Save(operationContext, process))
			if operationError := database.Instances().Unlink(operationContext, identifier); !errors.Is(operationError, store.ErrInstanceBusy) {
				test.Fatalf("running process was not protected: %v", operationError)
			}
			process.Status = "stopped"
			testutil.RequireNoError(test, database.Processes().Save(operationContext, process))
			testutil.RequireNoError(test, database.Instances().Unlink(operationContext, identifier))
			if _, operationError := database.Instances().Get(operationContext, identifier); !errors.Is(operationError, store.ErrInstanceNotFound) {
				test.Fatalf("unlinked reference is readable: %v", operationError)
			}
			linked, operationError := database.Instances().All(operationContext)
			testutil.RequireNoError(test, operationError)
			for _, current := range linked {
				if current.ID == identifier {
					test.Fatal("unlinked workspace returned in listing")
				}
			}
			if operationError := database.Instances().Save(operationContext, instance); !errors.Is(operationError, store.ErrInstanceUnlinked) {
				test.Fatalf("stale lifecycle save restored reference: %v", operationError)
			}
			lateSession := session
			lateSession.ID = atom.SessionID(identifier + "-late")
			if operationError := database.Sessions().Save(operationContext, lateSession); !errors.Is(operationError, store.ErrInstanceUnlinked) {
				test.Fatalf("late session writer succeeded: %v", operationError)
			}
			afterMessages, operationError := database.Sessions().Messages(operationContext, session.ID)
			testutil.RequireNoError(test, operationError)
			if !reflect.DeepEqual(beforeMessages, afterMessages) {
				test.Fatal("unlink changed saved messages")
			}
			afterUsage, operationError := database.Usage().Instance(operationContext, identifier)
			testutil.RequireNoError(test, operationError)
			if !reflect.DeepEqual(beforeUsage, afterUsage) {
				test.Fatal("unlink changed recorded usage")
			}
			configuration, operationError := database.Settings().Get(operationContext, identifier, "plugin.context")
			testutil.RequireNoError(test, operationError)
			if configuration != `{"enabled":true}` {
				test.Fatal("workspace configuration was deleted")
			}
			key, operationError := database.Secrets().InstanceKey(operationContext, identifier, "synthetic-provider")
			testutil.RequireNoError(test, operationError)
			if key != "saved-test-key" {
				test.Fatal("workspace key was deleted")
			}
			content, operationError := os.ReadFile(path)
			testutil.RequireNoError(test, operationError)
			if string(content) != "workspace content stays\n" {
				test.Fatal("workspace files changed")
			}
			// A newly constructed repository must still hide the reference.
			if backend == "postgres" {
				reopened, operationError := postgres.Open(operationContext, os.Getenv("MTT_TEST_DATABASE_URL"))
				testutil.RequireNoError(test, operationError)
				defer reopened.Close()
				if _, operationError = reopened.Instances().Get(operationContext, identifier); !errors.Is(operationError, store.ErrInstanceNotFound) {
					test.Fatal("restart restored unlinked reference")
				}
			}
		})
	}
}

func TestInstanceUnlinkWinsAgainstStaleLifecycleSaves(test *testing.T) {
	for _, backend := range []string{"memory", "postgres"} {
		test.Run(backend, func(test *testing.T) {
			operationContext := context.Background()
			var database store.Store = memory.New()
			if backend == "postgres" {
				url := os.Getenv("MTT_TEST_DATABASE_URL")
				if url == "" {
					test.Skip("MTT_TEST_DATABASE_URL is not set")
				}
				var operationError error
				database, operationError = postgres.Open(operationContext, url)
				testutil.RequireNoError(test, operationError)
			}
			defer database.Close()
			instance := atom.InstanceSpec{ID: fmt.Sprintf("unlink-race-%s-%d", backend, time.Now().UnixNano()), Workspace: test.TempDir(), CreatedAt: time.Now()}
			testutil.RequireNoError(test, database.Instances().Save(operationContext, instance))
			var writers sync.WaitGroup
			failures := make(chan error, 32)
			for index := 0; index < 16; index++ {
				writers.Add(1)
				go func() {
					defer writers.Done()
					operationError := database.Instances().Save(operationContext, instance)
					if operationError != nil && !errors.Is(operationError, store.ErrInstanceUnlinked) {
						failures <- operationError
					}
				}()
			}
			testutil.RequireNoError(test, database.Instances().Unlink(operationContext, instance.ID))
			writers.Wait()
			close(failures)
			for operationError := range failures {
				test.Fatal(operationError)
			}
			if _, operationError := database.Instances().Get(operationContext, instance.ID); !errors.Is(operationError, store.ErrInstanceNotFound) {
				test.Fatal("concurrent save resurrected workspace")
			}
		})
	}
}
