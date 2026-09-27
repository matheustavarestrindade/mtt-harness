package harness

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type contextKey int

const sessionKey contextKey = iota

func WithSession(ctx context.Context, session atom.Session) context.Context {
	return context.WithValue(ctx, sessionKey, session)
}

func SessionFrom(ctx context.Context) (atom.Session, bool) {
	session, ok := ctx.Value(sessionKey).(atom.Session)
	return session, ok
}
