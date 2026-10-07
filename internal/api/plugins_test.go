package api_test

import (
	"context"
	"encoding/json"
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

type configurableTestPlugin struct{}

func (configurableTestPlugin) Name() string                 { return "fixture" }
func (configurableTestPlugin) Version() string              { return "1" }
func (configurableTestPlugin) Setup(*harness.Harness) error { return nil }
func (configurableTestPlugin) ReadState(_ context.Context, workspace string) (harness.PluginState, error) {
	return harness.PluginState{Name: "fixture", Version: "1", WorkspaceID: workspace, Available: true, Configuration: json.RawMessage(`{"enabled":false}`), Override: json.RawMessage(`{}`), Schema: atom.Schema{JSON: json.RawMessage(`{"type":"object"}`)}}, nil
}
func (plugin configurableTestPlugin) UpdateConfiguration(operationContext context.Context, workspace string, input json.RawMessage) (harness.PluginState, error) {
	state, operationError := plugin.ReadState(operationContext, workspace)
	state.RequestedEnabled = true
	state.Pending = true
	state.Override = input
	return state, operationError
}
func (configurableTestPlugin) Metrics(_ context.Context, workspace string) (harness.PluginMetrics, error) {
	return harness.PluginMetrics{Name: "fixture", WorkspaceID: workspace, Counters: map[string]int64{"checks": 1}}, nil
}

func TestPluginAPIAuthenticatesAndPreservesPendingState(test *testing.T) {
	database := memory.New()
	testutil.RequireNoError(test, database.Instances().Save(context.Background(), atom.InstanceSpec{ID: "scope"}))
	host := plugins.New(harness.New())
	testutil.RequireNoError(test, host.Attach(configurableTestPlugin{}))
	server := httptest.NewServer(api.New(api.Config{Token: "secret", Store: database, Plugins: host}).Handler())
	defer server.Close()
	response := request(test, server.URL+"/plugins", "", "GET", nil)
	if response.StatusCode != http.StatusUnauthorized {
		test.Fatal("plugin API did not authenticate")
	}
	response.Body.Close()
	response = request(test, server.URL+"/instances/scope/plugins/fixture/settings", "secret", "PATCH", map[string]any{"enabled": true})
	if response.StatusCode != http.StatusOK {
		test.Fatalf("patch status: %d", response.StatusCode)
	}
	var state harness.PluginState
	decode(test, response, &state)
	if state.WorkspaceID != "scope" || state.Enabled || !state.Pending || !state.RequestedEnabled {
		test.Fatal("API lost requested/applied plugin state")
	}
	response = request(test, server.URL+"/instances/missing/plugins", "secret", "GET", nil)
	if response.StatusCode != http.StatusNotFound {
		test.Fatal("plugin API accepted a missing workspace")
	}
	response.Body.Close()
	response = request(test, server.URL+"/instances/scope/plugins/fixture/statistics", "secret", "GET", nil)
	var metrics harness.PluginMetrics
	decode(test, response, &metrics)
	if metrics.Counters["checks"] != 1 || metrics.WorkspaceID != "scope" {
		test.Fatal("wrong metrics scope")
	}
}
