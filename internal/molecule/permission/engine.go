package permission

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

type cacheKey struct {
	instanceID string
	sessionID  atom.SessionID
	target     string
}
type Engine struct {
	mutex sync.Mutex
	cache map[cacheKey]atom.PermissionDecision
	store store.PermissionStore
}

func NewEngine() *Engine {
	return &Engine{cache: map[cacheKey]atom.PermissionDecision{}}
}
func (permissionEngine *Engine) SetStore(permissionStore store.PermissionStore) {
	permissionEngine.mutex.Lock()
	defer permissionEngine.mutex.Unlock()
	permissionEngine.store = permissionStore
}

func ValidDecision(decision atom.PermissionDecision) bool {
	if decision.Kind != atom.VerdictAllow && decision.Kind != atom.VerdictDeny {
		return false
	}
	return decision.Scope == atom.ScopeOnce || decision.Scope == atom.ScopeSession || decision.Scope == atom.ScopeAlways
}

func (permissionEngine *Engine) Remember(operationContext context.Context, session atom.Session, target string, decision atom.PermissionDecision) error {
	if !ValidDecision(decision) {
		return fmt.Errorf("invalid permission decision")
	}
	if operationError := operationContext.Err(); operationError != nil {
		return operationError
	}
	decision.InstanceID, decision.SessionID, decision.Target = session.InstanceID, session.ID, target
	if decision.CreatedAt.IsZero() {
		decision.CreatedAt = time.Now()
	}
	permissionEngine.mutex.Lock()
	defer permissionEngine.mutex.Unlock()
	if permissionEngine.store != nil {
		return permissionEngine.store.Save(operationContext, decision)
	}
	if decision.Scope == atom.ScopeOnce {
		return nil
	}
	key := cacheKey{instanceID: session.InstanceID, target: target}
	if decision.Scope == atom.ScopeSession {
		key.sessionID = session.ID
	}
	permissionEngine.cache[key] = decision
	return nil
}

func (permissionEngine *Engine) Cached(operationContext context.Context, session atom.Session, target string) (atom.PermissionDecision, bool, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return atom.PermissionDecision{}, false, operationError
	}
	permissionEngine.mutex.Lock()
	defer permissionEngine.mutex.Unlock()
	if permissionEngine.store != nil {
		return permissionEngine.store.Resolve(operationContext, session, target)
	}
	if decision, found := permissionEngine.cache[cacheKey{session.InstanceID, session.ID, target}]; found {
		return decision, true, nil
	}
	decision, found := permissionEngine.cache[cacheKey{instanceID: session.InstanceID, target: target}]
	return decision, found, nil
}
