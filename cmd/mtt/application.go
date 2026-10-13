package main

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/api"
	"github.com/matheustavarestrindade/mtt-harness/internal/config"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/eventbus"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/permission"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/process"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/startprompt"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/toolsearch"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/plugins"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/processes"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/providerauth"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/registry"
)

func runApplication(configuration config.File) (operationError error) {
	startupPrompt, operationError := startprompt.Load(configuration.StartPromptFile)
	requireStartupSuccess(operationError, "load startup prompt")
	operationContext, cancelApplication := context.WithCancel(context.Background())
	defer cancelApplication()
	database := openStore(operationContext, configuration.DatabaseURL)
	defer database.Close()
	providerAuthentication := providerauth.New(operationContext, database.Secrets(), nil)
	defer providerAuthentication.Close()
	var messageQueue *loop.Queue
	var processManager *processes.Manager
	var toolSearch *toolsearch.Selector
	var pluginHost *plugins.Host
	var pluginEmbeddingCloser io.Closer
	closeMCP := func() error {
		return nil
	}
	defer func() {
		// HTTP has its own shutdown deadline. Resource owners must finish before
		// the deferred database close, even when an earlier caller stopped waiting.
		shutdownContext := context.Background()
		if messageQueue != nil {
			operationError = errors.Join(operationError, messageQueue.Close(shutdownContext))
		}
		if processManager != nil {
			operationError = errors.Join(operationError, processManager.Close(shutdownContext))
		}
		if pluginHost != nil {
			operationError = errors.Join(operationError, pluginHost.Close(shutdownContext))
		}
		if pluginEmbeddingCloser != nil {
			operationError = errors.Join(operationError, pluginEmbeddingCloser.Close())
		}
		operationError = errors.Join(operationError, closeMCP())
		if toolSearch != nil {
			operationError = errors.Join(operationError, toolSearch.Close())
		}
		cancelApplication()
	}()
	harnessRuntime := harness.New()
	pluginHost = plugins.New(harnessRuntime)
	eventBus := eventbus.New(harnessRuntime)
	eventBus.SetRecorder(database.Events().Record)
	token := resolveToken(operationContext, database, configuration.APIToken)
	toolSearch = loadToolSearch(operationContext, configuration.ProvidersFile)
	toolRegistry := registry.New(harnessRuntime, toolSearch)
	permissionEngine := permission.NewEngine()
	permissionBroker := permission.NewBroker()
	processSupervisor := process.New(0)
	modelGateway := gateway.New(harnessRuntime)
	instanceManager := instances.New(func(instanceID string) instances.SessionManager {
		return memory.New(instanceID, database.Sessions())
	}, database.Settings())
	processManager = processes.New(processSupervisor, database, harnessRuntime, eventBus, instanceManager)
	restoreInstances(operationContext, database, instanceManager)
	agentLoop := loop.New(harnessRuntime, loop.Config{
		Gateway: modelGateway, Registry: toolRegistry, Store: database, Bus: eventBus,
		Broker: permissionBroker, Engine: permissionEngine, Instances: instanceManager,
		StartPrompt: startupPrompt,
		Plugins:     pluginHost,
	})
	attachTools(toolRegistry, processManager, agentLoop)
	messageQueue = loop.NewQueue(agentLoop)
	processManager.SetNotifier(func(notificationContext context.Context, session atom.Session, content []atom.Content) error {
		_, _, operationError := messageQueue.SubmitRuntimeContent(notificationContext, session, content)
		if errors.Is(operationError, loop.ErrQueueClosed) || errors.Is(operationError, loop.ErrInstanceStopped) {
			return nil
		}
		return operationError
	})
	attachPlugins(pluginHost, instanceManager)
	if configuration.TestProvider {
		testProvider := provider.NewTest("test", provider.Text("the test provider is active"))
		requireStartupSuccess(modelGateway.Add(testProvider), "attach test provider")
		log.Printf("mtt: the test provider is in use")
	}
	loadProviders(operationContext, configuration.ProvidersFile, modelGateway, database, providerAuthentication)
	pluginEmbeddingCloser, operationError = attachContextPlugin(operationContext, pluginHost, database, instanceManager, modelGateway, configuration.DatabaseURL, configuration.ProvidersFile)
	requireStartupSuccess(operationError, "attach context plugin")
	requireStartupSuccess(attachSpacedRepetition(operationContext, pluginHost, database, instanceManager, modelGateway, agentLoop, configuration.DatabaseURL, configuration.SpacedRepetitionPrompts, startupPrompt), "attach spaced repetition plugin")
	requireStartupSuccess(attachSidekick(operationContext, pluginHost, database, instanceManager, modelGateway, agentLoop, configuration.DatabaseURL, configuration.SidekickPromptFile), "attach Sidekick plugin")
	mcpCleanup, mcpError := loadMCP(operationContext, configuration.MCPFile, toolRegistry)
	requireStartupSuccess(mcpError, "connect MCP servers")
	closeMCP = mcpCleanup
	requireStartupSuccess(messageQueue.Restore(operationContext), "restore pending messages")
	server := &http.Server{
		Addr: ":" + strconv.Itoa(configuration.Port),
		Handler: api.New(api.Config{
			Token: token, Store: database, Instances: instanceManager, Bus: eventBus,
			Broker: permissionBroker, Loop: agentLoop, Queue: messageQueue,
			Processes: processManager, Gateway: modelGateway,
			ProviderAuth: providerAuthentication,
			Plugins:      pluginHost,
		}).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		BaseContext: func(net.Listener) context.Context {
			return operationContext
		},
	}
	signalContext, stop := signal.NotifyContext(operationContext, os.Interrupt, syscall.SIGTERM)
	defer stop()
	shutdownDone := make(chan error, 1)
	go func() {
		<-signalContext.Done()
		cancelApplication()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownDone <- server.Shutdown(shutdownContext)
	}()
	log.Printf("mtt: the API is on port %d", configuration.Port)
	operationError = server.ListenAndServe()
	if errors.Is(operationError, http.ErrServerClosed) {
		return <-shutdownDone
	}
	return operationError
}
