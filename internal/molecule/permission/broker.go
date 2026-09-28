package permission

import (
	"context"
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type Broker struct {
	mutex     sync.Mutex
	pending   map[string]chan atom.PermissionDecision
	onRequest func(request atom.PermissionRequest)
}

func NewBroker() *Broker {
	return &Broker{pending: map[string]chan atom.PermissionDecision{}}
}

func (permissionBroker *Broker) SetRequestHandler(handler func(request atom.PermissionRequest)) {
	permissionBroker.mutex.Lock()
	defer permissionBroker.mutex.Unlock()
	permissionBroker.onRequest = handler
}

func (permissionBroker *Broker) Request(operationContext context.Context, request atom.PermissionRequest, onReady ...func() error) (atom.PermissionDecision, error) {
	channel := make(chan atom.PermissionDecision, 1)
	permissionBroker.mutex.Lock()
	permissionBroker.pending[request.ID] = channel
	handler := permissionBroker.onRequest
	permissionBroker.mutex.Unlock()
	defer func() {
		permissionBroker.mutex.Lock()
		defer permissionBroker.mutex.Unlock()
		delete(permissionBroker.pending, request.ID)
	}()
	for _, notify := range onReady {
		if operationError := notify(); operationError != nil {
			return atom.PermissionDecision{}, operationError
		}
	}
	if handler != nil {
		handler(request)
	}
	select {
	case decision := <-channel:
		return decision, nil
	case <-operationContext.Done():
		return atom.PermissionDecision{}, operationContext.Err()
	}
}

func (permissionBroker *Broker) Resolve(identifier string, decision atom.PermissionDecision) bool {
	if decision.Scope == "" {
		decision.Scope = atom.ScopeOnce
	}
	if !ValidDecision(decision) {
		return false
	}
	permissionBroker.mutex.Lock()
	channel, found := permissionBroker.pending[identifier]
	if found {
		delete(permissionBroker.pending, identifier)
	}
	permissionBroker.mutex.Unlock()
	if !found {
		return false
	}
	decision.RequestID = identifier
	channel <- decision
	return true
}

func (permissionBroker *Broker) Pending() int {
	permissionBroker.mutex.Lock()
	defer permissionBroker.mutex.Unlock()
	return len(permissionBroker.pending)
}
