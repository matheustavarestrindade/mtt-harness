package main

import (
	"context"
	"errors"

	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/eventbus"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/plugins"
	"github.com/matheustavarestrindade/mtt-harness/plugins/watcher"
)

func attachWatcher(operationContext context.Context, pluginHost *plugins.Host, database store.Store, instanceManager *instances.Manager, modelGateway *gateway.Gateway, agentLoop *loop.Loop, queue *loop.Queue, events *eventbus.Bus, databaseURL string) error {
	services := &plugins.Services{Gateway: modelGateway, Instances: instanceManager, Store: database}
	plugin, operationError := watcher.New(operationContext, watcher.Options{DatabaseURL: databaseURL, Services: harness.PluginServices{Settings: database.Settings(), Conversations: services, Workspaces: services, Models: services, Usage: database.Usage(), Tasks: services, Sessions: agentLoop.SessionWorkers(queue), Events: events}})
	if operationError != nil {
		return operationError
	}
	if operationError = pluginHost.Attach(plugin); operationError != nil {
		return errors.Join(operationError, plugin.Close(context.Background()))
	}
	return nil
}
