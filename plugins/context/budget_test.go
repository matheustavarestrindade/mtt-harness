package contextplugin

import (
	"context"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestThresholdNoticesFollowSelectedModelWithoutChangingPrefix(test *testing.T) {
	fixture := newPluginFixture(test)
	messages := fixtureMessages(fixture.session, 1500)
	initial := fixture.prepare(test, messages, 200000)
	measurement := measuredInput(fixture.session, initial.Request.Messages, 200000)
	budget, operationError := measurement.Measure(context.Background(), initial.Request)
	testutil.RequireNoError(test, operationError)
	for _, percentage := range []int{50, 65} {
		input := measuredInput(fixture.session, messages, budget.InputTokens*100/percentage)
		input.Request.Model = "selected-smaller-model"
		input.Model.ID = input.Request.Model
		selection, operationError := fixture.plugin.PrepareContext(context.Background(), input)
		testutil.RequireNoError(test, operationError)
		if snapshotText(selection.Request) != snapshotText(initial.Request) {
			test.Fatal("model budget notice rewrote the memory prefix")
		}
		last := selection.Request.Messages[len(selection.Request.Messages)-1]
		if last.Role != atom.RoleRuntime || !last.Ephemeral || !strings.Contains(last.Content[0].Text, "Context budget:") {
			test.Fatal("budget notice did not stay at the runtime tail")
		}
		expected := `"notice":"review"`
		if percentage == 65 {
			expected = `"notice":"urgent"`
		}
		if !strings.Contains(last.Content[0].Text, expected) {
			test.Fatalf("notice did not use the selected model budget: %s", last.Content[0].Text)
		}
	}
}

func TestOversizedProtectedInputDoesNotRemoveSourceMessages(test *testing.T) {
	fixture := newPluginFixture(test)
	messages := []atom.Message{testMessage("latest-user", atom.RoleUser, strings.Repeat("necessary input ", 1000))}
	_, operationError := fixture.plugin.PrepareContext(context.Background(), measuredInput(fixture.session, messages, 5000))
	if operationError == nil {
		test.Fatal("oversized protected input was sent")
	}
	if len(fixture.view(test).Removed) != 0 {
		test.Fatal("error silently removed protected data")
	}
}
