package memory

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

type processes struct{ store *Store }

func (processStore *processes) Recent(operationContext context.Context, session atom.SessionID, limit int) ([]atom.ProcessRecord, error) {
	if limit < 1 || limit > 256 {
		return nil, fmt.Errorf("process metadata limit must be between 1 and 256")
	}
	all, operationError := processStore.List(operationContext, session)
	if operationError != nil {
		return nil, operationError
	}
	slices.Reverse(all)
	if len(all) > limit {
		all = all[:limit]
	}
	for index := range all {
		all[index].Spec.Args = nil
		all[index].Spec.Env = nil
	}
	return all, nil
}

func (processStore *processes) ForMessages(operationContext context.Context, session atom.SessionID, messageIDs []string) ([]atom.ProcessRecord, error) {
	if len(messageIDs) > 101 {
		return nil, fmt.Errorf("process message limit is 101")
	}
	processStore.store.mutex.RLock()
	defer processStore.store.mutex.RUnlock()
	records := []atom.ProcessRecord{}
	for _, record := range processStore.store.processes {
		if record.SessionID == session && slices.Contains(messageIDs, record.Spec.ToolMessageID) {
			record.Spec.Args = nil
			record.Spec.Env = nil
			records = append(records, record)
			if len(records) == 1000 {
				break
			}
		}
	}
	return records, operationContext.Err()
}

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
