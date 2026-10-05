package memory

import (
	"context"
	"errors"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

type permissions struct{ store *Store }

func (permissionStore *permissions) Save(operationContext context.Context, decision atom.PermissionDecision) error {
	permissionStore.store.mutex.Lock()
	defer permissionStore.store.mutex.Unlock()
	if permissionStore.store.deletedSessions[decision.SessionID] {
		return store.ErrSessionDeleted
	}
	permissionStore.store.permissions[decision.RequestID] = decision
	return nil
}

func (permissionStore *permissions) Get(operationContext context.Context, identifier string) (atom.PermissionDecision, error) {
	permissionStore.store.mutex.RLock()
	defer permissionStore.store.mutex.RUnlock()
	decision, found := permissionStore.store.permissions[identifier]
	if !found {
		return atom.PermissionDecision{}, errors.New("memory: the decision is not in the store")
	}
	return decision, nil
}

func (permissionStore *permissions) Resolve(operationContext context.Context, session atom.Session, target string) (atom.PermissionDecision, bool, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return atom.PermissionDecision{}, false, operationError
	}
	permissionStore.store.mutex.RLock()
	defer permissionStore.store.mutex.RUnlock()
	var selected atom.PermissionDecision
	found := false
	for _, decision := range permissionStore.store.permissions {
		if decision.InstanceID != session.InstanceID || decision.Target != target {
			continue
		}
		if decision.Scope != atom.ScopeAlways && !(decision.Scope == atom.ScopeSession && decision.SessionID == session.ID) {
			continue
		}
		if found && selected.Scope == atom.ScopeSession && decision.Scope != atom.ScopeSession {
			continue
		}
		moreSpecific := found && selected.Scope == atom.ScopeAlways && decision.Scope == atom.ScopeSession
		if !found || moreSpecific || decision.CreatedAt.After(selected.CreatedAt) || (decision.CreatedAt.Equal(selected.CreatedAt) && decision.RequestID > selected.RequestID) {
			selected, found = decision, true
		}
	}
	return selected, found, nil
}
