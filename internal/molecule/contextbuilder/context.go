package contextbuilder

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

type Builder struct {
	store store.SessionStore
}

func New(s store.SessionStore) *Builder {
	return &Builder{store: s}
}

func (b *Builder) Build(ctx context.Context, session atom.SessionID) ([]atom.Message, error) {
	return b.store.Messages(ctx, session)
}
