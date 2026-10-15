package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/api"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/plugins"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

type activityTestPlugin struct{ configurableTestPlugin }

func (activityTestPlugin) SessionActivity(_ context.Context, session atom.Session, query harness.PluginActivityQuery) (harness.PluginActivityPage, error) {
	if query.Cursor == "changed" {
		return harness.PluginActivityPage{}, harness.ErrPluginActivityChanged
	}
	if query.Cursor == "invalid" {
		return harness.PluginActivityPage{}, harness.ErrPluginActivityCursor
	}
	return harness.PluginActivityPage{Name: "fixture", SessionID: session.ID, Revision: 7, HistoryRevision: 2, Entries: []harness.PluginActivityEntry{{ID: session.InstanceID, SessionID: session.ID, AnchorID: "anchor", Notes: []harness.PluginActivityNote{{Text: "Delivered reference data"}}}}}, nil
}

func TestPluginSessionActivityUsesAuthenticatedSessionScope(test *testing.T) {
	database := memory.New()
	testutil.RequireNoError(test, database.Sessions().Save(context.Background(), atom.Session{ID: "session", InstanceID: "workspace"}))
	host := plugins.New(harness.New())
	testutil.RequireNoError(test, host.Attach(activityTestPlugin{}))
	server := httptest.NewServer(api.New(api.Config{Token: "secret", Store: database, Plugins: host}).Handler())
	defer server.Close()
	path := server.URL + "/sessions/session/plugins/fixture/activity"
	response := request(test, path, "", "GET", nil)
	if response.StatusCode != http.StatusUnauthorized {
		test.Fatal("activity bypassed authentication")
	}
	response.Body.Close()
	response = request(test, path, "secret", "GET", nil)
	if response.StatusCode != http.StatusOK {
		test.Fatalf("activity status %d", response.StatusCode)
	}
	var page harness.PluginActivityPage
	decode(test, response, &page)
	if page.SessionID != "session" || page.Entries[0].ID != "workspace" || page.Revision != 7 || page.HistoryRevision != 2 {
		test.Fatalf("wrong session/plugin scope: %#v", page)
	}
	for suffix, status := range map[string]int{"?limit=0": 400, "?limit=101": 400, "?limit=bad": 400, "?cursor=invalid": 400, "?cursor=changed": 409} {
		response = request(test, path+suffix, "secret", "GET", nil)
		if response.StatusCode != status {
			test.Fatalf("invalid activity input %s gave %d", suffix, response.StatusCode)
		}
		response.Body.Close()
	}
	response = request(test, server.URL+"/sessions/missing/plugins/fixture/activity", "secret", "GET", nil)
	if response.StatusCode != http.StatusNotFound {
		test.Fatal("activity accepted a missing session")
	}
	response.Body.Close()
	response = request(test, server.URL+"/sessions/session/plugins/missing/activity", "secret", "GET", nil)
	if response.StatusCode != http.StatusNotFound {
		test.Fatal("activity accepted a missing plugin")
	}
	response.Body.Close()
}
