package store

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

// TaskStateStore owns atomic session task updates. Model-response accounting uses
// the revision observed before that request, so an update during streaming or its
// tool batch is never immediately counted as stale. Reads do not create records.
type TaskStateStore interface {
	Get(operationContext context.Context, sessionID atom.SessionID) (atom.TaskState, error)
	Update(operationContext context.Context, sessionID atom.SessionID, update atom.TaskStateUpdate) (atom.TaskState, error)
	RecordResponse(operationContext context.Context, sessionID atom.SessionID, revision int64) error
	Pause(operationContext context.Context, sessionID atom.SessionID) (atom.TaskState, bool, error)
}
