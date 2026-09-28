package memory

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type events struct{ store *Store }

func (eventStore *events) Append(operationContext context.Context, event atom.Event) error {
	_, operationError := eventStore.Record(operationContext, event)
	return operationError
}

func (eventStore *events) Record(operationContext context.Context, event atom.Event) (atom.Event, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return event, operationError
	}
	eventStore.store.mutex.Lock()
	defer eventStore.store.mutex.Unlock()
	eventStore.store.sequenceNumber++
	event.Seq = eventStore.store.sequenceNumber
	eventStore.store.events = append(eventStore.store.events, event)
	return event, nil
}

func (eventStore *events) Since(operationContext context.Context, instanceID string, sequenceNumber uint64) ([]atom.Event, error) {
	eventStore.store.mutex.RLock()
	defer eventStore.store.mutex.RUnlock()
	var list []atom.Event
	for _, event := range eventStore.store.events {
		if event.Seq > sequenceNumber && (instanceID == "" || event.InstanceID == instanceID) {
			list = append(list, event)
			if len(list) == 1000 {
				break
			}
		}
	}
	return list, nil
}
