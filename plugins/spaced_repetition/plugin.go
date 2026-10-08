package spacedrepetition

import (
	"context"
	"fmt"
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

// Options contains bootstrap-owned services and the loaded prompt fingerprint.
type Options struct {
	DatabaseURL   string
	PromptVersion string
	Services      harness.PluginServices
}

type workspaceRuntime struct {
	configuration configuration
	initialized   bool
	pending       *configuration
	requests      int
	changed       chan struct{}
	jobs          map[string]context.CancelFunc
	lastError     string
}

type requestState struct {
	workspaceID   string
	sessionID     atom.SessionID
	configuration configuration
	state         sessionState
	done          <-chan struct{}
}

type requestContextKey struct{ plugin *Plugin }

// Plugin owns only reminder state and bounded foreground recovery workers.
// Provider calls and memory searches never hold metadata or configuration locks.
type Plugin struct {
	services           harness.PluginServices
	database           repository
	promptVersion      string
	unavailable        error
	operationContext   context.Context
	cancel             context.CancelFunc
	mutex              sync.Mutex
	configurationMutex sync.Mutex
	scopes             map[string]*workspaceRuntime
	workers            sync.WaitGroup
	closed             bool
	unsubscribe        harness.Unsubscribe
}

func New(operationContext context.Context, options Options) (*Plugin, error) {
	if options.Services.Settings == nil || options.Services.Conversations == nil || options.Services.Models == nil || options.Services.Workspaces == nil || options.Services.Usage == nil || options.Services.Prompts == nil {
		return nil, fmt.Errorf("spaced repetition services are incomplete")
	}
	pluginContext, cancel := context.WithCancel(operationContext)
	plugin := &Plugin{services: options.Services, promptVersion: options.PromptVersion, operationContext: pluginContext, cancel: cancel, scopes: map[string]*workspaceRuntime{}}
	database, operationError := openRepository(operationContext, options.DatabaseURL)
	if operationError != nil {
		plugin.unavailable = operationError
	} else {
		plugin.database = database
	}
	return plugin, nil
}

func (plugin *Plugin) Name() string    { return pluginName }
func (plugin *Plugin) Version() string { return "0.1.0" }

func (plugin *Plugin) Setup(harnessRuntime *harness.Harness) error {
	plugin.mutex.Lock()
	defer plugin.mutex.Unlock()
	if plugin.closed || plugin.unsubscribe != nil {
		return fmt.Errorf("spaced repetition is already attached or closed")
	}
	if _, found := harnessRuntime.ToolByName("remember_instructions"); found {
		return fmt.Errorf("tool remember_instructions is already registered")
	}
	plugin.unsubscribe = harnessRuntime.Tool(&instructionTool{plugin: plugin})
	return nil
}

func (plugin *Plugin) Close(operationContext context.Context) error {
	plugin.mutex.Lock()
	if !plugin.closed {
		plugin.closed = true
		plugin.cancel()
		for _, scope := range plugin.scopes {
			for _, cancel := range scope.jobs {
				cancel()
			}
			close(scope.changed)
			scope.changed = make(chan struct{})
		}
	}
	plugin.mutex.Unlock()
	plugin.workers.Wait()
	plugin.mutex.Lock()
	defer plugin.mutex.Unlock()
	if plugin.unsubscribe != nil {
		plugin.unsubscribe()
		plugin.unsubscribe = nil
	}
	if plugin.database != nil {
		plugin.database.Close()
	}
	return nil
}

func (plugin *Plugin) scopeLocked(workspaceID string) *workspaceRuntime {
	scope := plugin.scopes[workspaceID]
	if scope == nil {
		scope = &workspaceRuntime{changed: make(chan struct{}), jobs: map[string]context.CancelFunc{}}
		plugin.scopes[workspaceID] = scope
	}
	return scope
}

func (plugin *Plugin) applyConfigurationLocked(scope *workspaceRuntime, value configuration) {
	scope.configuration = value
	scope.initialized = true
	scope.pending = nil
	if !value.Enabled {
		for _, cancel := range scope.jobs {
			cancel()
		}
	}
	close(scope.changed)
	scope.changed = make(chan struct{})
}

func (plugin *Plugin) validateSession(operationContext context.Context, session atom.Session) error {
	current, operationError := plugin.services.Conversations.Get(operationContext, session.ID)
	if operationError != nil {
		return operationError
	}
	if current.InstanceID != session.InstanceID {
		return fmt.Errorf("spaced repetition session belongs to another workspace")
	}
	return nil
}
