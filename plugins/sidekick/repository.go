package sidekick

import "context"

type repository interface {
	ReadSession(context.Context, string, string) (sessionState, error)
	UpdateSession(context.Context, string, string, func(*sessionState) (mutation, error)) (sessionState, error)
	Counters(context.Context, string) (map[string]int64, error)
	Close()
}
