package contextplugin

import (
	"context"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func (plugin *Plugin) Metrics(operationContext context.Context, workspaceID string) (harness.PluginMetrics, error) {
	result := harness.PluginMetrics{Name: pluginName, WorkspaceID: workspaceID, Counters: map[string]int64{}}
	if plugin.database != nil {
		counters, operationError := plugin.database.Count(operationContext, workspaceID)
		if operationError != nil {
			return result, operationError
		}
		result.Counters = counters
	}
	agents, operationError := plugin.services.Usage.Agents(operationContext, workspaceID)
	if operationError != nil {
		return result, operationError
	}
	for _, agent := range agents {
		if strings.HasPrefix(agent.Agent, "context.") {
			result.Agents = append(result.Agents, agent)
		}
	}
	return result, nil
}
