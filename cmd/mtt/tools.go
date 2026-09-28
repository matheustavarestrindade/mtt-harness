package main

import (
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/processes"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/registry"
	"github.com/matheustavarestrindade/mtt-harness/internal/tools"
)

func attachTools(toolRegistry *registry.Registry, processManager *processes.Manager, agentLoop *loop.Loop) {
	for _, tool := range []harness.Tool{
		tools.NewBash(processManager), tools.Read{}, tools.Write{}, tools.NewSearch(toolRegistry),
		tools.NewProcessOutput(processManager), tools.NewProcessKill(processManager),
		tools.Finish{}, &tools.Agent{RunTask: agentLoop.RunAgentTask},
	} {
		requireStartupSuccess(toolRegistry.Add(tool), "register tool "+tool.Name())
	}
}
