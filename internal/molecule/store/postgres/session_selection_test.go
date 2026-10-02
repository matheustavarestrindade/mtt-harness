package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestSessionSelectionRejectsStaleWriters(test *testing.T) {
	database := regressionStore(test)
	operationContext := context.Background()
	identifier := fmt.Sprintf("selection-%d", time.Now().UnixNano())
	session := atom.Session{ID: atom.SessionID(identifier), InstanceID: identifier, Model: "fixture/large", ReasoningEffort: "high", CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	selection := atom.SessionModelSelection{Model: "fixture/small"}
	testutil.RequireNoError(test, database.Sessions().SetModelSelection(operationContext, session.ID, session.ModelSelection(), selection))
	operationError := database.Sessions().SetModelSelection(operationContext, session.ID, session.ModelSelection(), atom.SessionModelSelection{Model: "fixture/large", ReasoningEffort: "low"})
	if !errors.Is(operationError, store.ErrSessionSelectionChanged) {
		test.Fatal("stale reasoning validation overwrote a model change")
	}
	session.Completed = true
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	loaded, found, operationError := database.Sessions().GetModelSelection(operationContext, session.ID)
	testutil.RequireNoError(test, operationError)
	if !found || loaded != selection {
		test.Fatalf("lifecycle save restored old selection: %+v", loaded)
	}
	operationError = database.Sessions().SetModelSelection(operationContext, session.ID, selection, session.ModelSelection())
	if !errors.Is(operationError, store.ErrSessionSelectionChanged) {
		test.Fatal("completed session accepted a model change")
	}
}
