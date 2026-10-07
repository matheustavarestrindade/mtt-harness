package loop

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/contextbuilder"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/eventbus"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/permission"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/pipeline"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/startprompt"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/operation"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/processes"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/registry"
)

const defaultMaxRounds = 64

type Config struct {
	Gateway     *gateway.Gateway
	Registry    *registry.Registry
	Store       store.Store
	Bus         *eventbus.Bus
	Broker      *permission.Broker
	Engine      *permission.Engine
	Processes   *processes.Manager
	Instances   *instances.Manager
	StartPrompt *startprompt.Template
	MaxRounds   int
	Plugins     harness.PluginRuntime
}

type Loop struct {
	harnessRuntime *harness.Harness
	pipeline       *pipeline.Pipeline
	configuration  Config
	mutex          sync.Mutex
	groups         map[atom.SessionID][]atom.ToolSpec
	finished       map[atom.SessionID]string
	runLocks       sync.Map
	activeRuns     map[atom.SessionID]*activeRun
	sessionBlocks  map[atom.SessionID]error
	contextBuilder *contextbuilder.Builder
}

func New(harnessRuntime *harness.Harness, configuration Config) *Loop {
	if configuration.Engine != nil && configuration.Store != nil {
		configuration.Engine.SetStore(configuration.Store.Permissions())
	}
	if configuration.MaxRounds <= 0 {
		configuration.MaxRounds = defaultMaxRounds
	}
	return &Loop{
		harnessRuntime: harnessRuntime,
		pipeline:       pipeline.New(harnessRuntime),
		configuration:  configuration,
		groups:         map[atom.SessionID][]atom.ToolSpec{},
		finished:       map[atom.SessionID]string{},
		activeRuns:     map[atom.SessionID]*activeRun{},
		sessionBlocks:  map[atom.SessionID]error{},
		contextBuilder: contextbuilder.New(configuration.Store.Sessions()),
	}
}

type activeRun struct {
	cancel  context.CancelFunc
	done    chan struct{}
	session atom.Session
}

// Run owns one session turn. Queue serializes user turns; child agents call Run
// with their own session. Cancellation is passed to providers, tools, and stores.
func (agentLoop *Loop) Run(operationContext context.Context, session atom.Session) error {
	return agentLoop.runSession(operationContext, session, "")
}

func (agentLoop *Loop) runSession(operationContext context.Context, session atom.Session, queuedMessageID string) (operationError error) {
	value, _ := agentLoop.runLocks.LoadOrStore(session.ID, make(chan struct{}, 1))
	lock := value.(chan struct{})
	select {
	case lock <- struct{}{}:
	case <-operationContext.Done():
		return operationContext.Err()
	}
	defer func() {
		<-lock
	}()
	operationContext, cancel := context.WithCancel(operationContext)
	defer cancel()
	active := &activeRun{cancel: cancel, done: make(chan struct{}), session: session}
	agentLoop.mutex.Lock()
	if blocked := agentLoop.sessionBlocks[session.ID]; blocked != nil {
		agentLoop.mutex.Unlock()
		return blocked
	}
	if blocked := agentLoop.sessionBlocks[session.Parent]; blocked != nil {
		agentLoop.mutex.Unlock()
		return blocked
	}
	agentLoop.activeRuns[session.ID] = active
	agentLoop.mutex.Unlock()
	defer func() {
		agentLoop.mutex.Lock()
		delete(agentLoop.activeRuns, session.ID)
		close(active.done)
		agentLoop.mutex.Unlock()
	}()
	operationContext = harness.WithSession(operationContext, session)
	if agentLoop.configuration.Instances != nil {
		instance, found := agentLoop.configuration.Instances.Get(session.InstanceID)
		if !found {
			return fmt.Errorf("instance %q is not running", session.InstanceID)
		}
		operationContext = harness.WithWorkspace(operationContext, instance.Workspace())
	}
	if queuedMessageID != "" {
		if operationError := agentLoop.configuration.Store.Queue().Start(operationContext, queuedMessageID); operationError != nil {
			return operationError
		}
		if session.Parent != "" {
			current, operationError := agentLoop.configuration.Store.Sessions().Get(operationContext, session.ID)
			if operationError != nil {
				return operationError
			}
			if current.Completed {
				return nil
			}
		}
	}
	defer func() {
		completionContext, stop := context.WithTimeout(context.WithoutCancel(operationContext), 5*time.Second)
		defer stop()
		if operationError == nil && session.Parent != "" {
			operationError = agentLoop.clearFinishedTaskState(completionContext, session)
		}
		status := "completed"
		if operationError != nil {
			status = "error"
			if errors.Is(operationError, context.Canceled) {
				status = "cancelled"
			}
			operationError = errors.Join(operationError, agentLoop.repairToolHistory(completionContext, session))
			operationError = errors.Join(operationError, agentLoop.pauseTaskState(completionContext, session))
		}
		if session.Parent != "" {
			session.Completed = true
			operationError = errors.Join(operationError, agentLoop.configuration.Store.Sessions().Save(completionContext, session))
		}
		if lifecycle, supported := agentLoop.configuration.Plugins.(harness.TurnLifecyclePlugin); supported {
			operationError = errors.Join(operationError, lifecycle.EndTurn(completionContext, session, status))
		}
		operationError = errors.Join(operationError, agentLoop.emitSessionEvent(completionContext, session, atom.EventTurnEnd, map[string]any{"status": status}))
	}()
	if operationError := agentLoop.emitSessionEvent(operationContext, session, atom.EventTurnStart, nil); operationError != nil {
		return operationError
	}
	for round := 0; round < agentLoop.configuration.MaxRounds; round++ {
		if operationError := operationContext.Err(); operationError != nil {
			return operationError
		}
		modelCall, operationError := agentLoop.prepareModelCall(operationContext, session)
		if operationError != nil {
			return operation.WrapError(operationError, "prepare model call")
		}
		if modelCall == nil {
			return nil
		}
		finished, operationError := agentLoop.runModelRound(session, modelCall)
		if operationError != nil || finished {
			return operationError
		}
	}
	return fmt.Errorf("model round limit reached")
}

func (agentLoop *Loop) CancelRun(sessionID atom.SessionID) bool {
	agentLoop.mutex.Lock()
	defer agentLoop.mutex.Unlock()
	active := agentLoop.activeRuns[sessionID]
	if active == nil {
		return false
	}
	active.cancel()
	return true
}

func (agentLoop *Loop) cancelAndWait(operationContext context.Context, sessionID atom.SessionID) error {
	agentLoop.mutex.Lock()
	active := agentLoop.activeRuns[sessionID]
	if active != nil {
		active.cancel()
	}
	agentLoop.mutex.Unlock()
	if active == nil {
		return nil
	}
	select {
	case <-active.done:
		return nil
	case <-operationContext.Done():
		return operationContext.Err()
	}
}

func (agentLoop *Loop) cancelInstance(operationContext context.Context, instanceID string) error {
	agentLoop.mutex.Lock()
	var runs []*activeRun
	for _, active := range agentLoop.activeRuns {
		if active.session.InstanceID == instanceID {
			active.cancel()
			runs = append(runs, active)
		}
	}
	agentLoop.mutex.Unlock()
	for _, active := range runs {
		select {
		case <-active.done:
		case <-operationContext.Done():
			return operationContext.Err()
		}
	}
	return nil
}
