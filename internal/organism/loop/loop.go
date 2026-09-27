package loop

import (
	"context"
	"errors"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/pipeline"
)

type Loop struct {
	h *harness.Harness
	p *pipeline.Pipeline
}

func New(h *harness.Harness) *Loop {
	return &Loop{h: h, p: pipeline.New(h)}
}

func (l *Loop) Run(ctx context.Context, session atom.Session) error {
	return errors.New("loop: not implemented")
}
