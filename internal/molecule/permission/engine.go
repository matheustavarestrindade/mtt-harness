package permission

import (
	"context"
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type Engine struct {
	mu    sync.Mutex
	cache map[string]atom.PermissionDecision
}

func New() *Engine {
	return &Engine{cache: map[string]atom.PermissionDecision{}}
}

func (e *Engine) Ask(ctx context.Context, request atom.PermissionRequest) (atom.PermissionDecision, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if decision, ok := e.cache[request.Target]; ok {
		decision.RequestID = request.ID
		return decision, nil
	}
	return atom.PermissionDecision{
		RequestID: request.ID,
		Kind:      atom.VerdictAsk,
		Scope:     atom.ScopeOnce,
	}, nil
}

func (e *Engine) Remember(decision atom.PermissionDecision) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cache[decision.RequestID] = decision
}

func (e *Engine) Cached(target string) (atom.PermissionDecision, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	decision, ok := e.cache[target]
	return decision, ok
}
