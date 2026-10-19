package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

func (eventStore *events) RecentSession(operationContext context.Context, sessionID atom.SessionID, limit int) ([]atom.Event, error) {
	if limit < 1 || limit > 1000 {
		return nil, fmt.Errorf("event limit must be between 1 and 1000")
	}
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	eventStore.store.mutex.RLock()
	defer eventStore.store.mutex.RUnlock()
	var result []atom.Event
	for index := len(eventStore.store.events) - 1; index >= 0 && len(result) < limit; index-- {
		event := eventStore.store.events[index]
		if event.SessionID == sessionID {
			if len(event.Payload) > 65536 {
				event.Payload, _ = json.Marshal(map[string]int{"omitted_bytes": len(event.Payload)})
			}
			result = append(result, event)
		}
	}
	slices.Reverse(result)
	return result, nil
}

type events struct{ store *Store }

func (eventStore *events) LatestSessionEvent(operationContext context.Context, sessionID atom.SessionID, name atom.EventName) (*atom.Event, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	eventStore.store.mutex.RLock()
	defer eventStore.store.mutex.RUnlock()
	for index := len(eventStore.store.events) - 1; index >= 0; index-- {
		event := eventStore.store.events[index]
		if event.SessionID == sessionID && event.Name == name {
			return &event, nil
		}
	}
	return nil, nil
}

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
	if eventStore.store.deletedSessions[event.SessionID] {
		return event, store.ErrSessionDeleted
	}
	eventStore.store.sequenceNumber++
	event.Seq = eventStore.store.sequenceNumber
	eventStore.store.events = append(eventStore.store.events, event)
	return event, nil
}

func (eventStore *events) Since(operationContext context.Context, instanceID string, sequenceNumber uint64) ([]atom.Event, error) {
	return eventStore.readEvents(operationContext, instanceID, nil, sequenceNumber)
}

func (eventStore *events) SinceSession(operationContext context.Context, sessionID atom.SessionID, sequenceNumber uint64) ([]atom.Event, error) {
	return eventStore.readEvents(operationContext, "", &sessionID, sequenceNumber)
}

func (eventStore *events) readEvents(operationContext context.Context, instanceID string, sessionID *atom.SessionID, sequenceNumber uint64) ([]atom.Event, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	eventStore.store.mutex.RLock()
	defer eventStore.store.mutex.RUnlock()
	var list []atom.Event
	for _, event := range eventStore.store.events {
		if event.Seq > sequenceNumber && (instanceID == "" || event.InstanceID == instanceID) && (sessionID == nil || event.SessionID == *sessionID) {
			list = append(list, event)
			if len(list) == 1000 {
				break
			}
		}
	}
	return list, nil
}
