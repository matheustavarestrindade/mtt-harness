package spacedrepetition

import (
	"context"
	"errors"
)

var errSessionRetired = errors.New("spaced repetition session is deleted or fenced")

// Metadata commits are short and serialize by session. Model calls, template
// rendering, and memory retrieval must run outside repository transactions.
type repository interface {
	ReadSession(context.Context, string, string) (sessionState, error)
	UpdateSession(context.Context, string, string, func(*sessionState) (map[string]int64, error)) (sessionState, error)
	ReadRecovery(context.Context, string, string) (recoveryRun, error)
	SaveRecovery(context.Context, recoveryRun) error
	CompleteRecovery(context.Context, recoveryRun, recoveryReport, map[string]int64) error
	DeleteRecoveries(context.Context, string, string) error
	Counters(context.Context, string) (map[string]int64, error)
	Close()
}
