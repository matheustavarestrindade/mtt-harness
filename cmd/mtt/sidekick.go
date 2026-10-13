package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"unicode/utf8"

	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/startprompt"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/plugins"
	"github.com/matheustavarestrindade/mtt-harness/plugins/sidekick"
)

func attachSidekick(operationContext context.Context, pluginHost *plugins.Host, database store.Store, instanceManager *instances.Manager, modelGateway *gateway.Gateway, agentLoop *loop.Loop, databaseURL, promptFile string) error {
	data, operationError := os.ReadFile(promptFile)
	if operationError != nil {
		return operationError
	}
	if len(data) == 0 || len(data) > 8192 || !utf8.Valid(data) {
		return fmt.Errorf("sidekick prompt is empty, exceeds 8192 bytes, or is not UTF-8")
	}
	template, operationError := startprompt.Parse(string(data))
	if operationError != nil {
		return operationError
	}
	digest := sha256.Sum256(data)
	services := &plugins.Services{Gateway: modelGateway, Instances: instanceManager, Store: database}
	var memory harness.WorkspaceMemory
	if registered, found := pluginHost.Get("context"); found {
		memory, _ = registered.(harness.WorkspaceMemory)
	}
	plugin, operationError := sidekick.New(operationContext, sidekick.Options{DatabaseURL: databaseURL, PromptVersion: hex.EncodeToString(digest[:]), Services: harness.PluginServices{Settings: database.Settings(), Conversations: services, Workspaces: services, Models: services, Usage: database.Usage(), Prompts: agentLoop.PromptRenderer(map[string]*startprompt.Template{"sidekick": template}), Memory: memory, Tasks: services, Files: services}})
	if operationError != nil {
		return operationError
	}
	if operationError := pluginHost.Attach(plugin); operationError != nil {
		return errors.Join(operationError, plugin.Close(context.Background()))
	}
	return nil
}
