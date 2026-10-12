package eventbus

import (
	"context"
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

// Recorder returns the event with its authoritative persisted sequence number.
type Recorder func(context.Context, atom.Event) (atom.Event, error)

// Bus owns persistence, while Harness owns every subscription (including API
// clients and plugins). Callbacks run outside the persistence lock.
type Bus struct {
	mutex          sync.Mutex
	harnessRuntime *harness.Harness
	recorder       Recorder
	sequenceNumber uint64
	sessions       map[atom.SessionID]*sessionGate
}

type sessionGate struct {
	ready chan struct{}
	users int
}

func New(harnessRuntime *harness.Harness) *Bus {
	return &Bus{harnessRuntime: harnessRuntime, sessions: make(map[atom.SessionID]*sessionGate)}
}

func (eventBus *Bus) SetRecorder(recorder Recorder) {
	eventBus.mutex.Lock()
	defer eventBus.mutex.Unlock()
	eventBus.recorder = recorder
}

func (eventBus *Bus) Send(operationContext context.Context, event atom.Event) error {
	event, operationError := eventBus.record(operationContext, event)
	if operationError != nil {
		return operationError
	}
	eventBus.harnessRuntime.Emit(operationContext, event)
	return nil
}

func (eventBus *Bus) record(operationContext context.Context, event atom.Event) (atom.Event, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return event, operationError
	}
	release, operationError := eventBus.acquireSession(operationContext, event.SessionID)
	if operationError != nil {
		return event, operationError
	}
	defer release()
	eventBus.mutex.Lock()
	recorder := eventBus.recorder
	eventBus.mutex.Unlock()
	if recorder != nil {
		persisted, operationError := recorder(operationContext, event)
		if operationError != nil {
			return event, operationError
		}
		eventBus.mutex.Lock()
		eventBus.sequenceNumber = max(eventBus.sequenceNumber, persisted.Seq)
		eventBus.mutex.Unlock()
		return persisted, nil
	}
	eventBus.mutex.Lock()
	eventBus.sequenceNumber++
	event.Seq = eventBus.sequenceNumber
	eventBus.mutex.Unlock()
	return event, nil
}

// Gates contain no workers and live only while a publisher or waiter uses them.
// Same-session persistence is ordered; unrelated sessions never wait for that
// I/O. Cancellation also releases queued admissions. Callbacks run after release.
func (eventBus *Bus) acquireSession(operationContext context.Context, sessionID atom.SessionID) (func(), error) {
	eventBus.mutex.Lock()
	gate := eventBus.sessions[sessionID]
	if gate == nil {
		gate = &sessionGate{ready: make(chan struct{}, 1)}
		gate.ready <- struct{}{}
		eventBus.sessions[sessionID] = gate
	}
	gate.users++
	eventBus.mutex.Unlock()
	retire := func() {
		eventBus.mutex.Lock()
		gate.users--
		if gate.users == 0 {
			delete(eventBus.sessions, sessionID)
		}
		eventBus.mutex.Unlock()
	}
	select {
	case <-operationContext.Done():
		retire()
		return nil, operationContext.Err()
	case <-gate.ready:
	}
	release := func() { gate.ready <- struct{}{}; retire() }
	if operationError := operationContext.Err(); operationError != nil {
		release()
		return nil, operationError
	}
	return release, nil
}

func (eventBus *Bus) On(name atom.EventName, handler harness.Handler) harness.Unsubscribe {
	return eventBus.harnessRuntime.On(name, handler)
}
func (eventBus *Bus) LastSequence() uint64 {
	eventBus.mutex.Lock()
	defer eventBus.mutex.Unlock()
	return eventBus.sequenceNumber
}
