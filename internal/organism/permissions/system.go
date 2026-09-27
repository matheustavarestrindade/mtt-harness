package permissions

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/permission"
)

type System struct {
	engine *permission.Engine
	broker *permission.Broker
}

func New(engine *permission.Engine, broker *permission.Broker) *System {
	return &System{engine: engine, broker: broker}
}

func (s *System) Ask(ctx context.Context, request atom.PermissionRequest) (atom.PermissionDecision, error) {
	if decision, ok := s.engine.Cached(request.Target); ok {
		decision.RequestID = request.ID
		return decision, nil
	}
	return s.broker.Request(ctx, request)
}

func (s *System) Remember(target string, decision atom.PermissionDecision) {
	if decision.Scope == atom.ScopeSession || decision.Scope == atom.ScopeAlways {
		s.engine.Remember(target, decision)
	}
}

func (s *System) Resolve(id string, decision atom.PermissionDecision) bool {
	return s.broker.Resolve(id, decision)
}
