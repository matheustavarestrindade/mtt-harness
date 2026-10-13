// Package sidekick prepares task-scoped context without blocking the main agent.
package sidekick

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

// Options contains bootstrap services and the loaded worker-template fingerprint.
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
	running       int
	lastError     string
}
type observation struct {
	session    atom.Session
	state      atom.TaskState
	generation uint64
}
type sessionWorker struct {
	context      context.Context
	latest       observation
	changed      chan struct{}
	done         chan struct{}
	cancel       context.CancelFunc
	activeCancel context.CancelFunc
}
type requestState struct {
	session       atom.Session
	configuration configuration
	state         sessionState
	done          <-chan struct{}
}
type requestContextKey struct{ plugin *Plugin }

type modelView struct {
	sourceUser string
	text       string
}

// Plugin owns bounded coalescing workers. Event callbacks only change local
// metadata and signal workers; all I/O and inference run outside metadata locks.
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
	jobs               map[atom.SessionID]*sessionWorker
	paused             map[atom.SessionID]bool
	views              map[atom.SessionID]modelView
	changed            chan struct{}
	workers            sync.WaitGroup
	closed             bool
	unsubscribe        harness.Unsubscribe
}

func New(operationContext context.Context, options Options) (*Plugin, error) {
	if options.Services.Settings == nil || options.Services.Conversations == nil || options.Services.Workspaces == nil || options.Services.Models == nil || options.Services.Tasks == nil || options.Services.Prompts == nil || options.Services.Usage == nil {
		return nil, fmt.Errorf("sidekick services are incomplete")
	}
	pluginContext, cancel := context.WithCancel(operationContext)
	plugin := &Plugin{services: options.Services, promptVersion: options.PromptVersion, operationContext: pluginContext, cancel: cancel, scopes: map[string]*workspaceRuntime{}, jobs: map[atom.SessionID]*sessionWorker{}, paused: map[atom.SessionID]bool{}, views: map[atom.SessionID]modelView{}, changed: make(chan struct{})}
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
		return fmt.Errorf("sidekick is already attached or closed")
	}
	plugin.unsubscribe = harnessRuntime.On(atom.EventTaskStateUpdated, plugin.observeTaskEvent)
	return nil
}
func (plugin *Plugin) Close(operationContext context.Context) error {
	plugin.mutex.Lock()
	if !plugin.closed {
		plugin.closed = true
		plugin.cancel()
		for _, job := range plugin.jobs {
			job.cancel()
		}
		plugin.signalChangedLocked()
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
		scope = &workspaceRuntime{}
		plugin.scopes[workspaceID] = scope
	}
	return scope
}
func (plugin *Plugin) signalChangedLocked() {
	close(plugin.changed)
	plugin.changed = make(chan struct{})
}
func (plugin *Plugin) recordError(workspaceID string, operationError error) {
	if operationError == nil {
		return
	}
	plugin.mutex.Lock()
	plugin.scopeLocked(workspaceID).lastError = operationError.Error()
	plugin.mutex.Unlock()
}

func (plugin *Plugin) observeTaskEvent(operationContext context.Context, event atom.Event) {
	var state atom.TaskState
	if json.Unmarshal(event.Payload, &state) != nil || state.SessionID != event.SessionID {
		return
	}
	session, found := harness.SessionFrom(operationContext)
	if !found || session.ID != event.SessionID {
		session = atom.Session{ID: event.SessionID, InstanceID: event.InstanceID}
	}
	if session.InstanceID != event.InstanceID {
		return
	}
	plugin.observeTaskState(session, state)
}

func (plugin *Plugin) observeTaskState(session atom.Session, state atom.TaskState) {
	state.Todo = append([]atom.TaskItem(nil), state.Todo...)
	if state.Doing != nil {
		doing := *state.Doing
		state.Doing = &doing
	}
	plugin.mutex.Lock()
	defer plugin.mutex.Unlock()
	if plugin.closed || plugin.database == nil || plugin.paused[session.ID] {
		return
	}
	job := plugin.jobs[session.ID]
	if job != nil && job.context.Err() != nil {
		delete(plugin.jobs, session.ID)
		job = nil
	}
	if job != nil {
		if state.Revision < job.latest.state.Revision {
			return
		}
		if doingFingerprint(state.Doing) == doingFingerprint(job.latest.state.Doing) {
			job.latest.state = state
			return
		}
		job.latest = observation{session: session, state: state, generation: job.latest.generation + 1}
		if job.activeCancel != nil {
			job.activeCancel()
		}
		select {
		case job.changed <- struct{}{}:
		default:
		}
		return
	}
	scope := plugin.scopeLocked(session.InstanceID)
	if !scope.initialized || !scope.configuration.Enabled || state.Doing == nil || len(plugin.jobs) >= 128 {
		return
	}
	workerContext, cancel := context.WithCancel(plugin.operationContext)
	job = &sessionWorker{context: workerContext, latest: observation{session: session, state: state, generation: 1}, changed: make(chan struct{}, 1), done: make(chan struct{}), cancel: cancel}
	plugin.jobs[session.ID] = job
	plugin.workers.Add(1)
	go plugin.runSessionWorker(workerContext, job)
}
