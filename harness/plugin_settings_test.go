package harness_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type configurationReader struct {
	values  map[string]string
	failure error
	reads   []string
}

func (reader *configurationReader) Get(operationContext context.Context, workspaceID, key string) (string, error) {
	reader.reads = append(reader.reads, workspaceID+":"+key)
	return reader.values[workspaceID], reader.failure
}
func (*configurationReader) Save(context.Context, string, string, string) error { return nil }
func (*configurationReader) Delete(context.Context, string, string) error       { return nil }

func TestPluginConfigurationRetainsLayerOwnershipAndRawOverrides(test *testing.T) {
	reader := &configurationReader{values: map[string]string{"": `{"global":true}`, "workspace": `{"enabled":false}`}}
	var capturedGlobal, capturedWorkspace string
	merge := func(global, workspace json.RawMessage) (bool, error) {
		capturedGlobal, capturedWorkspace = string(global), string(workspace)
		return true, nil
	}
	_, raw, operationError := harness.LoadPluginConfiguration(context.Background(), reader, "workspace", "plugin.example", merge)
	if operationError != nil || capturedGlobal != reader.values[""] || capturedWorkspace != reader.values["workspace"] || string(raw) != reader.values["workspace"] {
		test.Fatal("configuration layers changed")
	}
	reader.values = map[string]string{}
	_, raw, operationError = harness.LoadPluginConfiguration(context.Background(), reader, "", "plugin.example", merge)
	if operationError != nil || string(raw) != "{}" || capturedWorkspace != "" {
		test.Fatal("global defaults lost their empty override")
	}
	reader.failure = errors.New("settings unavailable")
	_, _, operationError = harness.LoadPluginConfiguration(context.Background(), reader, "workspace", "plugin.example", merge)
	if !errors.Is(operationError, reader.failure) {
		test.Fatal("settings failure was swallowed")
	}
}
