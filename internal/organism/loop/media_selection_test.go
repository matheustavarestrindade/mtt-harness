package loop_test

import (
	"context"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestModelSwitchValidatesOnlyRetainedMedia(test *testing.T) {
	testStack := newStack(test, provider.Text("current reply"))
	testStack.harnessRuntime.Provider(switchingTestProvider{testStack.provider})
	session := testStack.instance(test, 2)
	oldMessage := atom.Message{ID: "old-image", SessionID: session.ID, Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Image, Data: []byte("old media"), MIME: "image/png"}}, CreatedAt: time.Now()}
	testutil.RequireNoError(test, testStack.database.Sessions().Append(context.Background(), oldMessage))
	testStack.user(test, session, "Continue with text")
	testStack.selectModel(test, &session, "test/small-model")
	testutil.RequireNoError(test, testStack.loop.Run(context.Background(), session))
	if len(testStack.provider.Requests) != 1 {
		test.Fatalf("provider calls: %d", len(testStack.provider.Requests))
	}
	for _, message := range testStack.provider.Requests[0].Messages {
		if message.ID == oldMessage.ID {
			test.Fatal("omitted image reached the text-only provider")
		}
	}
	history, operationError := testStack.database.Sessions().Messages(context.Background(), session.ID)
	testutil.RequireNoError(test, operationError)
	if history[0].ID != oldMessage.ID {
		test.Fatal("context fitting removed persisted history")
	}
}

func TestRetainedUnsupportedMediaStillPreventsDispatch(test *testing.T) {
	testStack := newStack(test, provider.Text("must not run"))
	session := testStack.instance(test, 2)
	message := atom.Message{ID: "current-image", SessionID: session.ID, Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Image, Data: []byte("new media"), MIME: "image/png"}}, CreatedAt: time.Now()}
	testutil.RequireNoError(test, testStack.database.Sessions().Append(context.Background(), message))
	if operationError := testStack.loop.Run(context.Background(), session); operationError == nil {
		test.Fatal("unsupported current input was accepted")
	}
	if len(testStack.provider.Requests) != 0 {
		test.Fatal("unsupported media reached the provider")
	}
}
