package contextbuilder

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

type Builder struct {
	store store.SessionStore
}

func New(sessionStore store.SessionStore) *Builder {
	return &Builder{store: sessionStore}
}

func (builder *Builder) Build(operationContext context.Context, session atom.SessionID) ([]atom.Message, error) {
	return builder.store.Messages(operationContext, session)
}
