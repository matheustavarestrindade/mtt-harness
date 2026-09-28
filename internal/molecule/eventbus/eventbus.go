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
}

func New(harnessRuntime *harness.Harness) *Bus {
	return &Bus{harnessRuntime: harnessRuntime}
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
	eventBus.mutex.Lock()
	defer eventBus.mutex.Unlock()
	if eventBus.recorder != nil {
		persisted, operationError := eventBus.recorder(operationContext, event)
		if operationError != nil {
			return event, operationError
		}
		eventBus.sequenceNumber = persisted.Seq
		return persisted, nil
	}
	eventBus.sequenceNumber++
	event.Seq = eventBus.sequenceNumber
	return event, nil
}

func (eventBus *Bus) On(name atom.EventName, handler harness.Handler) harness.Unsubscribe {
	return eventBus.harnessRuntime.On(name, handler)
}
func (eventBus *Bus) LastSequence() uint64 {
	eventBus.mutex.Lock()
	defer eventBus.mutex.Unlock()
	return eventBus.sequenceNumber
}
