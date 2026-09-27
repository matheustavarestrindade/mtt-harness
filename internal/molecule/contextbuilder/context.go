package contextbuilder

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type Builder struct{}

func New() *Builder {
	return &Builder{}
}

func (b *Builder) Build(ctx context.Context, session atom.SessionID) ([]atom.Message, error) {
	return nil, nil
}
