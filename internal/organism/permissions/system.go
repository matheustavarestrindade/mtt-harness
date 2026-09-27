package permissions

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/permission"
)

type System struct {
	engine *permission.Engine
}

func New(engine *permission.Engine) *System {
	return &System{engine: engine}
}

func (s *System) Ask(ctx context.Context, request atom.PermissionRequest) (atom.PermissionDecision, error) {
	return s.engine.Ask(ctx, request)
}

func (s *System) Remember(decision atom.PermissionDecision) {
	s.engine.Remember(decision)
}
