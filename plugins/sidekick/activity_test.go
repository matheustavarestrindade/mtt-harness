package sidekick

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestActivityPublishesOnlyCommittedHintsAndReadsWithoutWorkers(test *testing.T) {
	for _, postgres := range []bool{false, true} {
		test.Run(fmt.Sprint(postgres), func(test *testing.T) {
			fixture := newFixture(test, postgres)
			operationContext, release := fixture.begin(test)
			defer release()
			fixture.waitState(test, func(state sessionState) bool { return state.Pending != nil })
			query := harness.PluginActivityQuery{Limit: 50}
			before, operationError := fixture.plugin.SessionActivity(operationContext, fixture.session, query)
			testutil.RequireNoError(test, operationError)
			if len(before.Entries) != 0 {
				test.Fatal("prepared hint was exposed as delivered")
			}
			contribution, operationError := fixture.plugin.PrepareReminder(operationContext, fixture.request())
			testutil.RequireNoError(test, operationError)
			testutil.RequireNoError(test, contribution.Deferred(operationContext))
			deferred, operationError := fixture.plugin.SessionActivity(operationContext, fixture.session, query)
			testutil.RequireNoError(test, operationError)
			if len(deferred.Entries) != 0 {
				test.Fatal("budget-deferred hint was exposed")
			}
			testutil.RequireNoError(test, contribution.Commit(operationContext, harness.ReminderDelivery{Tokens: 37, Estimated: true}))
			page, operationError := fixture.plugin.SessionActivity(operationContext, fixture.session, query)
			testutil.RequireNoError(test, operationError)
			if len(page.Entries) != 1 {
				test.Fatalf("missing committed hint: %#v", page)
			}
			entry := page.Entries[0]
			if entry.SessionID != fixture.session.ID || entry.AnchorID != "user" || entry.Title != "Inspect persistence" || entry.InputTokens == nil || *entry.InputTokens != 37 || !entry.Estimated || len(entry.Notes) != 1 || entry.Notes[0].Text != "Use PostgreSQL for transactional storage." || len(entry.Notes[0].References) != 1 {
				test.Fatalf("incorrect public hint: %#v", entry)
			}
			encoded, _ := json.Marshal(page)
			if strings.Contains(string(encoded), "<sidekick>") || strings.Contains(string(encoded), "configuration_hash") || strings.Contains(string(encoded), "Do not repeat this metadata") {
				test.Fatal("private envelope leaked into activity notes")
			}
			fixture.services.mutex.Lock()
			calls := len(fixture.services.calls)
			fixture.services.mutex.Unlock()
			repeated, operationError := fixture.plugin.SessionActivity(operationContext, fixture.session, query)
			testutil.RequireNoError(test, operationError)
			fixture.services.mutex.Lock()
			afterCalls := len(fixture.services.calls)
			fixture.services.mutex.Unlock()
			if repeated.Revision != page.Revision || afterCalls != calls {
				test.Fatal("read-only activity changed state or started inference")
			}
			// Request projection retirement must not erase the delivered history.
			_, operationError = fixture.plugin.database.UpdateSession(operationContext, fixture.session.InstanceID, string(fixture.session.ID), func(state *sessionState) (mutation, error) { state.Events = nil; return mutation{}, nil })
			testutil.RequireNoError(test, operationError)
			retired, operationError := fixture.plugin.SessionActivity(operationContext, fixture.session, query)
			testutil.RequireNoError(test, operationError)
			if len(retired.Entries) != 1 || retired.Entries[0].ID != entry.ID {
				test.Fatal("retiring a projection erased the delivered hint")
			}
		})
	}
}

func seedActivityHint(test *testing.T, fixture *fixture, identifier, anchor string, sequence int64, createdAt time.Time) {
	test.Helper()
	encoded, _ := json.Marshal(hintPayload{Doing: "Inspect sources", Notes: []deliveredNote{{Text: identifier, References: []string{"file: notes.md:1-2"}}}})
	event := hintEvent{ID: identifier, Anchor: anchor, AnchorSequence: sequence, Text: hintPrefix + string(encoded) + hintSuffix, CreatedAt: createdAt}
	_, operationError := fixture.plugin.database.UpdateSession(context.Background(), fixture.session.InstanceID, string(fixture.session.ID), func(state *sessionState) (mutation, error) {
		return mutation{Run: &workerRun{ID: identifier, WorkspaceID: fixture.session.InstanceID, SessionID: fixture.session.ID, Status: "ready", StartedAt: createdAt}}, nil
	})
	testutil.RequireNoError(test, operationError)
	_, operationError = fixture.plugin.database.UpdateSession(context.Background(), fixture.session.InstanceID, string(fixture.session.ID), func(state *sessionState) (mutation, error) {
		state.Events = append(state.Events, event)
		return mutation{RunID: identifier, RunStatus: "delivered", Hint: &event}, nil
	})
	testutil.RequireNoError(test, operationError)
}

func TestActivityPaginationLegacyRecoveryAndHistoryFences(test *testing.T) {
	for _, postgres := range []bool{false, true} {
		test.Run(fmt.Sprint(postgres), func(test *testing.T) {
			fixture := newFixture(test, postgres)
			operationContext := context.Background()
			createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			seedActivityHint(test, fixture, "first", "user", 1, createdAt)
			seedActivityHint(test, fixture, "second", "later", 2, createdAt.Add(time.Nanosecond))
			page, operationError := fixture.plugin.SessionActivity(operationContext, fixture.session, harness.PluginActivityQuery{Limit: 1})
			testutil.RequireNoError(test, operationError)
			if len(page.Entries) != 1 || page.Entries[0].ID != "second" || page.NextCursor == "" {
				test.Fatalf("pagination order/bound failed: %#v", page)
			}
			next, operationError := fixture.plugin.SessionActivity(operationContext, fixture.session, harness.PluginActivityQuery{Limit: 1, Cursor: page.NextCursor})
			testutil.RequireNoError(test, operationError)
			if len(next.Entries) != 1 || next.Entries[0].ID != "first" || next.NextCursor != "" {
				test.Fatal("page repeated or skipped a delivered hint")
			}
			foreign := fixture.session
			foreign.InstanceID = "other-workspace"
			if _, operationError = fixture.plugin.SessionActivity(operationContext, foreign, harness.PluginActivityQuery{Limit: 1, Cursor: page.NextCursor}); !errors.Is(operationError, harness.ErrPluginActivityCursor) {
				test.Fatal("cursor crossed workspace scope")
			}
			messages := []atom.Message{{ID: "user", SessionID: fixture.session.ID, Seq: 1, Role: atom.RoleUser}, {ID: "later", SessionID: fixture.session.ID, Seq: 2, Role: atom.RoleAssistant}}
			finish, operationError := fixture.plugin.BeginHistoryChange(operationContext, harness.HistoryChange{Kind: harness.HistoryRevert, WorkspaceID: fixture.session.InstanceID, KeepMessageID: "user", Snapshots: []harness.HistorySnapshot{{Session: fixture.session, Messages: messages}}})
			testutil.RequireNoError(test, operationError)
			if _, operationError = fixture.plugin.SessionActivity(operationContext, fixture.session, harness.PluginActivityQuery{Limit: 50}); !errors.Is(operationError, harness.ErrPluginActivityChanged) {
				test.Fatal("fenced history was exposed")
			}
			testutil.RequireNoError(test, finish(operationContext, true))
			if _, operationError = fixture.plugin.SessionActivity(operationContext, fixture.session, harness.PluginActivityQuery{Limit: 1, Cursor: page.NextCursor}); !errors.Is(operationError, harness.ErrPluginActivityChanged) {
				test.Fatal("old cursor survived history mutation")
			}
			retained, operationError := fixture.plugin.SessionActivity(operationContext, fixture.session, harness.PluginActivityQuery{Limit: 50})
			testutil.RequireNoError(test, operationError)
			if len(retained.Entries) != 1 || retained.Entries[0].ID != "first" || retained.HistoryRevision <= page.HistoryRevision {
				test.Fatalf("revert did not retain only preceding activity: %#v", retained)
			}
			// Older deployments have only an active event, without a run hint.
			_, operationError = fixture.plugin.database.UpdateSession(operationContext, fixture.session.InstanceID, string(fixture.session.ID), func(state *sessionState) (mutation, error) {
				state.Events = []hintEvent{{ID: "legacy", Anchor: "user", Text: "Legacy quoted hint", CreatedAt: createdAt.Add(time.Second)}}
				state.ActivityRecorded = false
				return mutation{}, nil
			})
			testutil.RequireNoError(test, operationError)
			legacy, operationError := fixture.plugin.SessionActivity(operationContext, fixture.session, harness.PluginActivityQuery{Limit: 50})
			testutil.RequireNoError(test, operationError)
			if len(legacy.Entries) != 2 || legacy.Entries[0].Notes[0].Text != "Legacy quoted hint" {
				test.Fatal("legacy hint was lost or invented")
			}
			legacyState, operationError := fixture.plugin.database.ReadSession(operationContext, fixture.session.InstanceID, string(fixture.session.ID))
			testutil.RequireNoError(test, operationError)
			if legacyState.ActivityRecorded {
				test.Fatal("read-only activity performed a legacy migration")
			}
			_, operationError = fixture.plugin.database.UpdateSession(operationContext, fixture.session.InstanceID, string(fixture.session.ID), func(state *sessionState) (mutation, error) { state.Events = nil; return mutation{}, nil })
			testutil.RequireNoError(test, operationError)
			migrated, operationError := fixture.plugin.SessionActivity(operationContext, fixture.session, harness.PluginActivityQuery{Limit: 50})
			testutil.RequireNoError(test, operationError)
			if len(migrated.Entries) != 2 {
				test.Fatal("legacy history was lost when its projection retired")
			}
			finish, operationError = fixture.plugin.BeginHistoryChange(operationContext, harness.HistoryChange{Kind: harness.HistoryDelete, WorkspaceID: fixture.session.InstanceID, Snapshots: []harness.HistorySnapshot{{Session: fixture.session, Messages: messages}}})
			testutil.RequireNoError(test, operationError)
			testutil.RequireNoError(test, finish(operationContext, true))
			deleted, operationError := fixture.plugin.SessionActivity(operationContext, fixture.session, harness.PluginActivityQuery{Limit: 50})
			testutil.RequireNoError(test, operationError)
			if len(deleted.Entries) != 0 {
				test.Fatal("deleted session activity remained public")
			}
			counters, operationError := fixture.plugin.database.Counters(operationContext, fixture.session.InstanceID)
			testutil.RequireNoError(test, operationError)
			if counters["runs/delivered"] != 0 {
				test.Fatal("deleted session retained delivered run records")
			}
		})
	}
}
