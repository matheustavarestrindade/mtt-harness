package memory

import (
	"context"
	"sort"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

type instances struct{ store *Store }

func (instanceStore *instances) Save(operationContext context.Context, instanceSpec atom.InstanceSpec) error {
	instanceStore.store.mutex.Lock()
	defer instanceStore.store.mutex.Unlock()
	if instanceStore.store.unlinkedInstances[instanceSpec.ID] {
		return store.ErrInstanceUnlinked
	}
	instanceStore.store.instances[instanceSpec.ID] = instanceSpec
	return nil
}

func (instanceStore *instances) Get(operationContext context.Context, identifier string) (atom.InstanceSpec, error) {
	instanceStore.store.mutex.RLock()
	defer instanceStore.store.mutex.RUnlock()
	instanceSpec, found := instanceStore.store.instances[identifier]
	if !found || instanceStore.store.unlinkedInstances[identifier] {
		return atom.InstanceSpec{}, store.ErrInstanceNotFound
	}
	return instanceSpec, nil
}

func (instanceStore *instances) All(operationContext context.Context) ([]atom.InstanceSpec, error) {
	instanceStore.store.mutex.RLock()
	defer instanceStore.store.mutex.RUnlock()
	list := make([]atom.InstanceSpec, 0, len(instanceStore.store.instances))
	for _, instanceSpec := range instanceStore.store.instances {
		if instanceStore.store.unlinkedInstances[instanceSpec.ID] {
			continue
		}
		list = append(list, instanceSpec)
	}
	sort.Slice(list, func(firstIndex, secondIndex int) bool {
		return list[firstIndex].ID < list[secondIndex].ID
	})
	return list, nil
}

func (instanceStore *instances) Delete(operationContext context.Context, identifier string) error {
	instanceStore.store.mutex.Lock()
	defer instanceStore.store.mutex.Unlock()
	delete(instanceStore.store.instances, identifier)
	return nil
}

func (instanceStore *instances) Unlink(operationContext context.Context, identifier string) error {
	if operationError := operationContext.Err(); operationError != nil {
		return operationError
	}
	instanceStore.store.mutex.Lock()
	defer instanceStore.store.mutex.Unlock()
	instanceSpec, found := instanceStore.store.instances[identifier]
	if !found || instanceStore.store.unlinkedInstances[identifier] {
		return store.ErrInstanceNotFound
	}
	for _, entry := range instanceStore.store.queued {
		if instanceStore.store.sessions[entry.Message.SessionID].InstanceID == identifier {
			return store.ErrInstanceBusy
		}
	}
	for _, process := range instanceStore.store.processes {
		if process.InstanceID == identifier && process.Status == "running" {
			return store.ErrInstanceBusy
		}
	}
	instanceSpec.Stopped = true
	instanceStore.store.instances[identifier] = instanceSpec
	instanceStore.store.unlinkedInstances[identifier] = true
	return nil
}
