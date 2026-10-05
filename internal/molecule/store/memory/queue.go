package memory

import (
	"context"
	"fmt"
	"sort"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

type queueStore struct{ store *Store }

func (queue *queueStore) Enqueue(operationContext context.Context, message atom.Message, limit int) error {
	queue.store.mutex.Lock()
	defer queue.store.mutex.Unlock()
	if operationError := operationContext.Err(); operationError != nil {
		return operationError
	}
	if queue.store.deletedSessions[message.SessionID] {
		return store.ErrSessionDeleted
	}
	count := 0
	for _, entry := range queue.store.queued {
		if entry.Message.SessionID == message.SessionID && !entry.Running {
			count++
		}
	}
	if count >= limit || len(queue.store.queued) >= 4096 {
		return store.ErrQueueFull
	}
	if _, duplicate := queue.store.queued[message.ID]; duplicate {
		return fmt.Errorf("duplicate queued message")
	}
	queue.store.queueSequence++
	queue.store.queued[message.ID] = atom.QueuedMessage{Message: message, Sequence: queue.store.queueSequence}
	return nil
}

func (queue *queueStore) All(operationContext context.Context) ([]atom.QueuedMessage, error) {
	queue.store.mutex.RLock()
	defer queue.store.mutex.RUnlock()
	var entries []atom.QueuedMessage
	for _, entry := range queue.store.queued {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(first, second int) bool {
		return entries[first].Sequence < entries[second].Sequence
	})
	return entries, operationContext.Err()
}

func (queue *queueStore) Start(operationContext context.Context, messageID string) error {
	queue.store.mutex.Lock()
	defer queue.store.mutex.Unlock()
	if operationError := operationContext.Err(); operationError != nil {
		return operationError
	}
	entry, found := queue.store.queued[messageID]
	if !found || entry.Running {
		return fmt.Errorf("message is not pending")
	}
	entry.Running = true
	queue.store.queued[messageID] = entry
	queue.store.messages[entry.Message.SessionID] = append(queue.store.messages[entry.Message.SessionID], entry.Message)
	return nil
}

func (queue *queueStore) Finish(operationContext context.Context, messageID string) error {
	queue.store.mutex.Lock()
	defer queue.store.mutex.Unlock()
	if operationError := operationContext.Err(); operationError != nil {
		return operationError
	}
	delete(queue.store.queued, messageID)
	return nil
}

func (queue *queueStore) Remove(operationContext context.Context, sessionID atom.SessionID, messageID string) (bool, error) {
	queue.store.mutex.Lock()
	defer queue.store.mutex.Unlock()
	if operationError := operationContext.Err(); operationError != nil {
		return false, operationError
	}
	entry, found := queue.store.queued[messageID]
	if !found || entry.Running || entry.Message.SessionID != sessionID {
		return false, nil
	}
	delete(queue.store.queued, messageID)
	return true, nil
}

func (queue *queueStore) ClearPending(operationContext context.Context, sessionID atom.SessionID) (int, error) {
	queue.store.mutex.Lock()
	defer queue.store.mutex.Unlock()
	if operationError := operationContext.Err(); operationError != nil {
		return 0, operationError
	}
	count := 0
	for identifier, entry := range queue.store.queued {
		if entry.Message.SessionID == sessionID && !entry.Running {
			delete(queue.store.queued, identifier)
			count++
		}
	}
	return count, nil
}
