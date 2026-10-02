package processes

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/eventbus"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/process"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

type InstanceLimits interface {
	ProcessLimit(operationContext context.Context, instanceID string) (int, error)
}
type Notifier func(context.Context, atom.Session, []atom.Content) error

// Manager owns process observation independently from the originating turn.
// Starts and instance limits are serialized; Close joins every observer before
// its context or database can be released.
type Manager struct {
	mutex          sync.Mutex
	supervisor     *process.Supervisor
	store          store.Store
	harnessRuntime *harness.Harness
	bus            *eventbus.Bus
	limits         InstanceLimits
	instances      map[string]string
	lifecycle      context.Context
	cancel         context.CancelFunc
	observers      sync.WaitGroup
	closing        bool
	notifier       Notifier
	starting       map[string]int
	starts         sync.WaitGroup
	instanceStarts map[string]*sync.WaitGroup
}

func New(supervisor *process.Supervisor, database store.Store, harnessRuntime *harness.Harness, bus *eventbus.Bus, limits InstanceLimits) *Manager {
	lifecycle, cancel := context.WithCancel(context.Background())
	return &Manager{supervisor: supervisor, store: database, harnessRuntime: harnessRuntime, bus: bus, limits: limits, instances: map[string]string{}, lifecycle: lifecycle, cancel: cancel, starting: map[string]int{}, instanceStarts: map[string]*sync.WaitGroup{}}
}

func (processManager *Manager) SetNotifier(notifier Notifier) {
	processManager.mutex.Lock()
	defer processManager.mutex.Unlock()
	processManager.notifier = notifier
}

func (processManager *Manager) Start(operationContext context.Context, processSpec atom.ProcessSpec) (harness.Process, error) {
	if processSpec.Notify.Mode == "" {
		processSpec.Notify.Mode = atom.NotifyExit
	}
	if processSpec.Notify.Mode != atom.NotifyNone && processSpec.Notify.Mode != atom.NotifyExit && processSpec.Notify.Mode != atom.NotifyError && processSpec.Notify.Mode != atom.NotifyInterval {
		return nil, fmt.Errorf("invalid process notification mode")
	}
	if processSpec.Notify.Mode == atom.NotifyInterval && processSpec.Notify.Interval <= 0 {
		return nil, fmt.Errorf("interval notifications require a positive interval")
	}
	if processSpec.Timeout < 0 {
		return nil, fmt.Errorf("process timeout cannot be negative")
	}
	session, _ := harness.SessionFrom(operationContext)
	workspace, _ := harness.WorkspaceFrom(operationContext)
	if processSpec.Cwd == "" {
		processSpec.Cwd = workspace
	}
	releaseReservation, operationError := processManager.reserveStart(operationContext, session.InstanceID)
	if operationError != nil {
		return nil, operationError
	}
	defer releaseReservation()
	observerContext := harness.WithWorkspace(harness.WithSession(processManager.lifecycle, session), workspace)
	runningProcess, operationError := processManager.supervisor.StartWithTransform(operationContext, processSpec, func(event atom.ProcessEvent) (atom.ProcessEvent, error) {
		return harness.Run(observerContext, processManager.harnessRuntime, atom.StageProcessOutput, event)
	})
	if operationError != nil {
		return nil, operationError
	}
	record := atom.ProcessRecord{ID: runningProcess.ID(), InstanceID: session.InstanceID, SessionID: session.ID, Spec: processSpec, PID: runningProcess.PID(), Status: "running", StartedAt: time.Now()}
	if operationError := processManager.store.Processes().Save(operationContext, record); operationError != nil {
		killError := runningProcess.Kill("")
		_, _ = runningProcess.Wait()
		return nil, errors.Join(operationError, killError)
	}
	processManager.mutex.Lock()
	processManager.instances[runningProcess.ID()] = session.InstanceID
	processManager.mutex.Unlock()
	processManager.observers.Add(1)
	go processManager.observeProcessEvents(observerContext, session, runningProcess, record)
	return runningProcess, nil
}

func (processManager *Manager) StopInstance(operationContext context.Context, instanceID string) (int, error) {
	processManager.mutex.Lock()
	starts := processManager.instanceStarts[instanceID]
	processManager.mutex.Unlock()
	if starts != nil {
		if operationError := waitForProcessStarts(operationContext, starts); operationError != nil {
			return 0, operationError
		}
	}
	processManager.mutex.Lock()
	var identifiers []string
	for identifier, owner := range processManager.instances {
		if owner == instanceID {
			identifiers = append(identifiers, identifier)
		}
	}
	processManager.mutex.Unlock()
	var failures []error
	for _, identifier := range identifiers {
		runningProcess, found := processManager.supervisor.Get(identifier)
		if !found {
			continue
		}
		failures = append(failures, runningProcess.Kill(""))
		done := make(chan struct{})
		go func() {
			_, _ = runningProcess.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-operationContext.Done():
			return len(identifiers), errors.Join(append(failures, operationContext.Err())...)
		}
	}
	return len(identifiers), errors.Join(failures...)
}

func (processManager *Manager) Get(identifier string) (harness.Process, bool) {
	return processManager.supervisor.Get(identifier)
}
func (processManager *Manager) All() []harness.Process {
	return processManager.supervisor.All()
}
func (processManager *Manager) Output(identifier string) ([]byte, []byte, bool) {
	runningProcess, found := processManager.Get(identifier)
	if !found {
		return nil, nil, false
	}
	stdout, stderr := runningProcess.(*process.Handle).Output()
	return stdout, stderr, true
}
func (processManager *Manager) Subscribe(identifier string) (<-chan atom.ProcessEvent, bool) {
	return processManager.SubscribeContext(context.Background(), identifier)
}
func (processManager *Manager) SubscribeContext(operationContext context.Context, identifier string) (<-chan atom.ProcessEvent, bool) {
	runningProcess, found := processManager.Get(identifier)
	if !found {
		return nil, false
	}
	return runningProcess.(*process.Handle).SubscribeContext(operationContext), true
}

func (processManager *Manager) CheckOwner(operationContext context.Context, identifier string) error {
	session, found := harness.SessionFrom(operationContext)
	if !found {
		return fmt.Errorf("process operation has no session")
	}
	record, operationError := processManager.store.Processes().Get(operationContext, identifier)
	if operationError != nil {
		return operationError
	}
	if record.InstanceID != session.InstanceID {
		return fmt.Errorf("process belongs to another instance")
	}
	return nil
}

func (processManager *Manager) Close(operationContext context.Context) error {
	processManager.mutex.Lock()
	processManager.closing = true
	processManager.mutex.Unlock()
	operationError := waitForProcessStarts(operationContext, &processManager.starts)
	operationError = errors.Join(operationError, processManager.supervisor.Close(operationContext))
	done := make(chan struct{})
	go func() {
		processManager.observers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-operationContext.Done():
		operationError = errors.Join(operationError, operationContext.Err())
	}
	processManager.cancel()
	return operationError
}

// Reserve before launching, but invoke plugin callbacks outside manager locks.
// Shutdown joins reservations before joining observers, preventing Add/Wait races.
func (processManager *Manager) reserveStart(operationContext context.Context, instanceID string) (func(), error) {
	processManager.mutex.Lock()
	defer processManager.mutex.Unlock()
	if processManager.closing {
		return nil, fmt.Errorf("process manager is closing")
	}
	if instanceID != "" && processManager.limits != nil {
		if owner, supported := processManager.limits.(interface{ IsRunning(string) bool }); supported && !owner.IsRunning(instanceID) {
			return nil, fmt.Errorf("instance is stopped")
		}
		limit, operationError := processManager.limits.ProcessLimit(operationContext, instanceID)
		if operationError != nil {
			return nil, operationError
		}
		count := processManager.starting[instanceID]
		for identifier, owner := range processManager.instances {
			if owner != instanceID {
				continue
			}
			if running, found := processManager.supervisor.Get(identifier); found && running.(*process.Handle).Running() {
				count++
			}
		}
		if count >= limit {
			return nil, fmt.Errorf("process limit of the instance is reached")
		}
	}
	processManager.starting[instanceID]++
	processManager.starts.Add(1)
	starts := processManager.instanceStarts[instanceID]
	if starts == nil {
		starts = &sync.WaitGroup{}
		processManager.instanceStarts[instanceID] = starts
	}
	starts.Add(1)
	return func() {
		processManager.mutex.Lock()
		defer processManager.mutex.Unlock()
		processManager.starting[instanceID]--
		starts.Done()
		processManager.starts.Done()
	}, nil
}

func waitForProcessStarts(operationContext context.Context, group *sync.WaitGroup) error {
	done := make(chan struct{})
	go func() {
		group.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-operationContext.Done():
		return operationContext.Err()
	}
}
