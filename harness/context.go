package harness

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type contextKey int

const (
	sessionKey contextKey = iota
	workspaceKey
)

func WithSession(operationContext context.Context, session atom.Session) context.Context {
	return context.WithValue(operationContext, sessionKey, session)
}

func SessionFrom(operationContext context.Context) (atom.Session, bool) {
	session, found := operationContext.Value(sessionKey).(atom.Session)
	return session, found
}

func WithWorkspace(operationContext context.Context, workspace string) context.Context {
	return context.WithValue(operationContext, workspaceKey, workspace)
}

func WorkspaceFrom(operationContext context.Context) (string, bool) {
	workspace, found := operationContext.Value(workspaceKey).(string)
	return workspace, found && workspace != ""
}
