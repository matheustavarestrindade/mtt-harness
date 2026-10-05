package memory

import (
	"context"
	"errors"
	"sort"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

type processes struct{ store *Store }

func (processStore *processes) Save(operationContext context.Context, process atom.ProcessRecord) error {
	processStore.store.mutex.Lock()
	defer processStore.store.mutex.Unlock()
	if processStore.store.deletedSessions[process.SessionID] {
		return store.ErrSessionDeleted
	}
	processStore.store.processes[process.ID] = process
	return nil
}

func (processStore *processes) Get(operationContext context.Context, identifier string) (atom.ProcessRecord, error) {
	processStore.store.mutex.RLock()
	defer processStore.store.mutex.RUnlock()
	record, found := processStore.store.processes[identifier]
	if !found {
		return atom.ProcessRecord{}, errors.New("memory: the process is not in the store")
	}
	return record, nil
}

func (processStore *processes) CountRunning(operationContext context.Context, instanceID string) (int, error) {
	processStore.store.mutex.RLock()
	defer processStore.store.mutex.RUnlock()
	count := 0
	for _, record := range processStore.store.processes {
		if record.InstanceID == instanceID && record.Status == "running" {
			count++
		}
	}
	return count, nil
}

func (processStore *processes) List(operationContext context.Context, session atom.SessionID) ([]atom.ProcessRecord, error) {
	processStore.store.mutex.RLock()
	defer processStore.store.mutex.RUnlock()
	var list []atom.ProcessRecord
	for _, record := range processStore.store.processes {
		if record.SessionID == session {
			list = append(list, record)
		}
	}
	sort.Slice(list, func(firstIndex, secondIndex int) bool {
		return list[firstIndex].StartedAt.Before(list[secondIndex].StartedAt)
	})
	return list, nil
}
