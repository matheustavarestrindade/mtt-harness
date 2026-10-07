package contextplugin

import (
	"context"
	"fmt"
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

// Options carries bootstrap-owned dependencies. Runtime settings and provider
// credentials stay in their existing database owners.
type Options struct {
	DatabaseURL    string
	Services       harness.PluginServices
	EmbeddingModel string
}

// Plugin owns workers and request leases. Inference never runs under the
// configuration or workspace mutex, or inside a database transaction.
type Plugin struct {
	services           harness.PluginServices
	database           repository
	embeddingModel     string
	processID          string
	unavailable        error
	operationContext   context.Context
	cancel             context.CancelFunc
	mutex              sync.Mutex
	configurationMutex sync.Mutex
	scopes             map[string]*workspaceRuntime
	workers            sync.WaitGroup
	dispatchDone       chan struct{}
	wake               chan struct{}
	closed             bool
	started            bool
	lastError          string
	unsubscribers      []harness.Unsubscribe
}

type workspaceRuntime struct {
	configuration configuration
	initialized   bool
	pending       *configuration
	requests      int
	changed       chan struct{}
	jobs          map[string]context.CancelFunc
	modelGate     chan struct{}
	lastHistorian int64
}

type requestState struct {
	workspaceID   string
	sessionID     atom.SessionID
	configuration configuration
	done          <-chan struct{}
}
type requestContextKey struct{ plugin *Plugin }

func New(operationContext context.Context, options Options) (*Plugin, error) {
	if options.Services.Settings == nil || options.Services.Models == nil || options.Services.Conversations == nil || options.Services.Workspaces == nil || options.Services.Usage == nil {
		return nil, fmt.Errorf("context plugin services are incomplete")
	}
	pluginContext, cancel := context.WithCancel(operationContext)
	plugin := &Plugin{services: options.Services, embeddingModel: options.EmbeddingModel, processID: newIdentifier(), operationContext: pluginContext, cancel: cancel, scopes: map[string]*workspaceRuntime{}, dispatchDone: make(chan struct{}), wake: make(chan struct{}, 1)}
	database, operationError := openRepository(operationContext, options.DatabaseURL)
	if operationError != nil {
		plugin.unavailable = operationError
	} else {
		plugin.database = database
	}
	return plugin, nil
}

func (plugin *Plugin) Name() string    { return pluginName }
func (plugin *Plugin) Version() string { return pluginVersion }

func (plugin *Plugin) Setup(harnessRuntime *harness.Harness) error {
	plugin.mutex.Lock()
	if plugin.closed || plugin.started {
		plugin.mutex.Unlock()
		return fmt.Errorf("context plugin is already started or closed")
	}
	plugin.mutex.Unlock()
	for _, name := range []string{"ctx_drop", "ctx_wrapup", "remember", "search_memory", "list_memory_categories"} {
		if _, found := harnessRuntime.ToolByName(name); found {
			return fmt.Errorf("tool %q is already registered", name)
		}
	}
	for _, name := range []string{"ctx_drop", "ctx_wrapup", "remember", "search_memory", "list_memory_categories"} {
		plugin.unsubscribers = append(plugin.unsubscribers, harnessRuntime.Tool(&memoryTool{plugin: plugin, name: name}))
	}
	plugin.mutex.Lock()
	plugin.started = true
	plugin.mutex.Unlock()
	go plugin.runDispatcher()
	return nil
}

func (plugin *Plugin) Close(operationContext context.Context) error {
	plugin.mutex.Lock()
	if !plugin.closed {
		plugin.closed = true
		plugin.cancel()
		if !plugin.started {
			close(plugin.dispatchDone)
		}
		for _, scope := range plugin.scopes {
			for _, cancel := range scope.jobs {
				cancel()
			}
			close(scope.changed)
			scope.changed = make(chan struct{})
		}
	}
	plugin.mutex.Unlock()
	<-plugin.dispatchDone
	plugin.workers.Wait()
	plugin.mutex.Lock()
	defer plugin.mutex.Unlock()
	for _, unsubscribe := range plugin.unsubscribers {
		unsubscribe()
	}
	plugin.unsubscribers = nil
	if plugin.database != nil {
		plugin.database.Close()
	}
	return nil
}

func (plugin *Plugin) signalWork() {
	select {
	case plugin.wake <- struct{}{}:
	default:
	}
}

func (plugin *Plugin) scopeLocked(workspaceID string) *workspaceRuntime {
	scope := plugin.scopes[workspaceID]
	if scope == nil {
		scope = &workspaceRuntime{changed: make(chan struct{}), jobs: map[string]context.CancelFunc{}, modelGate: make(chan struct{}, 1)}
		plugin.scopes[workspaceID] = scope
	}
	return scope
}

func (plugin *Plugin) applyConfigurationLocked(scope *workspaceRuntime, configuration configuration) {
	scope.configuration = configuration
	scope.initialized = true
	scope.pending = nil
	if !configuration.Enabled {
		for _, cancel := range scope.jobs {
			cancel()
		}
	}
	close(scope.changed)
	scope.changed = make(chan struct{})
}
