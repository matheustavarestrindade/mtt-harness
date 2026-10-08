package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/startprompt"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/plugins"
	spacedrepetition "github.com/matheustavarestrindade/mtt-harness/plugins/spaced_repetition"
)

func attachSpacedRepetition(operationContext context.Context, pluginHost *plugins.Host, database store.Store, instanceManager *instances.Manager, modelGateway *gateway.Gateway, agentLoop *loop.Loop, databaseURL, promptDirectory string, startup *startprompt.Template) error {
	templates := map[string]*startprompt.Template{"startup": startup}
	digest := sha256.New()
	_, _ = digest.Write([]byte(startup.Fingerprint()))
	for _, level := range []string{"low", "medium", "high"} {
		path := filepath.Join(promptDirectory, level+".md")
		data, operationError := os.ReadFile(path)
		if operationError != nil {
			return operationError
		}
		if len(data) == 0 || len(data) > 8192 || !utf8.Valid(data) {
			return fmt.Errorf("reminder prompt %q is empty, exceeds 8192 bytes, or is not UTF-8", path)
		}
		template, operationError := startprompt.Parse(string(data))
		if operationError != nil {
			return fmt.Errorf("reminder prompt %q: %w", path, operationError)
		}
		templates["spaced_repetition."+level] = template
		_, _ = digest.Write([]byte(level + "\x00"))
		_, _ = digest.Write(data)
	}
	services := &plugins.Services{Gateway: modelGateway, Instances: instanceManager, Store: database}
	var memory harness.WorkspaceMemory
	if registered, found := pluginHost.Get("context"); found {
		memory, _ = registered.(harness.WorkspaceMemory)
	}
	plugin, operationError := spacedrepetition.New(operationContext, spacedrepetition.Options{DatabaseURL: databaseURL, PromptVersion: hex.EncodeToString(digest.Sum(nil)), Services: harness.PluginServices{Settings: database.Settings(), Conversations: services, Workspaces: services, Models: services, Usage: database.Usage(), Prompts: agentLoop.PromptRenderer(templates), Memory: memory}})
	if operationError != nil {
		return operationError
	}
	if operationError := pluginHost.Attach(plugin); operationError != nil {
		return errors.Join(operationError, plugin.Close(context.Background()))
	}
	return nil
}
