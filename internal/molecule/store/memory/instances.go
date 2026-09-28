package memory

import (
	"context"
	"errors"
	"sort"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type instances struct{ store *Store }

func (instanceStore *instances) Save(operationContext context.Context, instanceSpec atom.InstanceSpec) error {
	instanceStore.store.mutex.Lock()
	defer instanceStore.store.mutex.Unlock()
	instanceStore.store.instances[instanceSpec.ID] = instanceSpec
	return nil
}

func (instanceStore *instances) Get(operationContext context.Context, identifier string) (atom.InstanceSpec, error) {
	instanceStore.store.mutex.RLock()
	defer instanceStore.store.mutex.RUnlock()
	instanceSpec, found := instanceStore.store.instances[identifier]
	if !found {
		return atom.InstanceSpec{}, errors.New("memory: the instance is not in the store")
	}
	return instanceSpec, nil
}

func (instanceStore *instances) All(operationContext context.Context) ([]atom.InstanceSpec, error) {
	instanceStore.store.mutex.RLock()
	defer instanceStore.store.mutex.RUnlock()
	list := make([]atom.InstanceSpec, 0, len(instanceStore.store.instances))
	for _, instanceSpec := range instanceStore.store.instances {
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
