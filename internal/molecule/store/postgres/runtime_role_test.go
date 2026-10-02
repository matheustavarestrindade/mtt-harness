package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestRuntimeOriginSurvivesPostgresQueueRecovery(test *testing.T) {
	database := regressionStore(test)
	operationContext, cancelOperation := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelOperation()
	identifier := fmt.Sprintf("runtime-origin-%d", time.Now().UnixNano())
	session := atom.Session{ID: atom.SessionID(identifier), InstanceID: identifier, CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	message := atom.Message{ID: identifier, SessionID: session.ID, Role: atom.RoleRuntime, Content: []atom.Content{{Type: atom.Text, Text: "background process output"}}, CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Queue().Enqueue(operationContext, message, 1))
	entries, operationError := database.Queue().All(operationContext)
	testutil.RequireNoError(test, operationError)
	found := false
	for _, entry := range entries {
		if entry.Message.ID != identifier {
			continue
		}
		found = true
		if entry.Message.Role != atom.RoleRuntime {
			test.Fatalf("restored queue entry impersonates the user: %+v", entry.Message)
		}
	}
	if !found {
		test.Fatal("runtime update was not durably queued")
	}
	testutil.RequireNoError(test, database.Queue().Start(operationContext, message.ID))
	messages, operationError := database.Sessions().Messages(operationContext, session.ID)
	testutil.RequireNoError(test, operationError)
	if len(messages) != 1 || messages[0].Role != atom.RoleRuntime || messages[0].Content[0].Text != message.Content[0].Text {
		test.Fatalf("history lost runtime origin: %+v", messages)
	}
	testutil.RequireNoError(test, database.Queue().Finish(operationContext, message.ID))
}
