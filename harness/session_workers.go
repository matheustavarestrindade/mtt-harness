package harness

import (
	"context"
	"errors"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

var ErrReviewStale = errors.New("review source changed")

// SessionControlToken belongs to a channel-owned coordinator incarnation.
// User admission, cancellation and lifecycle changes invalidate older tokens.
type SessionControlToken struct {
	Epoch       string
	Revision    uint64
	PendingUser bool
}
type ReviewFence struct {
	Control         SessionControlToken
	HistoryRevision uint64
	TaskRevision    int64
	UserMessageID   string
}

// ReviewSource contains bounded public conversation data. Private continuation
// state and media bytes never enter a worker prompt through this contract.
type ReviewSource struct {
	Session       atom.Session
	Workspace     atom.InstanceSpec
	Tasks         atom.TaskState
	Messages      []atom.Message
	Fence         ReviewFence
	HeadSequence  int64
	Truncated     bool
	Active        bool
	LiveMessageID string
}
type WorkerTool struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Categories  []string `json:"categories"`
	Operations  []string `json:"operations,omitempty"`
}
type WorkerSessionRequest struct {
	WorkspaceID     string
	SourceSessionID atom.SessionID
	Agent           string
	RunID           string
	Model           string
	ReasoningEffort string
	Prompt          string
	Tools           []atom.ToolGrant
	MaxRounds       int
	MaxOutputTokens int
	Started         func(atom.SessionID) error
}
type WorkerSessionResult struct {
	SessionID atom.SessionID
	Decision  string
	Message   string
	Reason    string
}

// SessionWorkers exposes existing loop, queue and permission owners. It is not a
// second provider runner. Source reads and model execution run in plugin workers.
type SessionWorkers interface {
	Tools(context.Context, string) ([]WorkerTool, error)
	Source(context.Context, atom.SessionID, int, bool) (ReviewSource, error)
	Current(context.Context, ReviewSource) (bool, error)
	Observe(context.Context, atom.Session) (<-chan struct{}, func(), error)
	Permission(context.Context, atom.SessionID, string) (*atom.PermissionRequest, error)
	Measure(context.Context, string, string, string, string, int) (RequestBudget, error)
	RunSession(context.Context, WorkerSessionRequest) (WorkerSessionResult, error)
	ResolvePermission(context.Context, ReviewSource, string, bool, string, string) (bool, error)
	SubmitContinuation(context.Context, ReviewSource, string, string) (bool, error)
}
