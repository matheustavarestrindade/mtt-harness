package main

import (
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/plugins"
	"github.com/matheustavarestrindade/mtt-harness/plugins/pathtools"
)

func attachPlugins(harnessRuntime *harness.Harness, instanceManager *instances.Manager) {
	pluginHost := plugins.New(harnessRuntime)
	pathGuard := &pathtools.PathGuard{WorkspaceOf: func(instanceID string) string {
		instance, found := instanceManager.Get(instanceID)
		if !found {
			return ""
		}
		return instance.Workspace()
	}}
	requireStartupSuccess(pluginHost.Attach(pathGuard), "attach path guard")
}
