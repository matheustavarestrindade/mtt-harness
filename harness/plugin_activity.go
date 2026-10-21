package harness

import (
	"context"
	"errors"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

var ErrPluginActivityCursor = errors.New("plugin activity cursor is invalid")
var ErrPluginActivityChanged = errors.New("plugin activity history changed")

// PluginActivityQuery requests a bounded, newest-first page. Cursor is opaque
// and scoped to one plugin/session history. Limit is 1–100; the API defaults to 50.
type PluginActivityQuery struct {
	Cursor string
	Limit  int
}

// PluginActivityNote exposes reference data, never worker reasoning or private
// provider state. References are display text, not commands or trusted URLs.
type PluginActivityNote struct {
	Text       string
	References []string
}

// PluginActivityEntry records committed plugin activity or a review outcome.
// An optional anchor belongs to saved history. Entries are not chat messages or
// tool calls; workers' private reasoning is never activity data.
type PluginActivityEntry struct {
	ID              string
	SessionID       atom.SessionID
	AnchorID        string
	AnchorSequence  int64
	Title           string
	Notes           []PluginActivityNote
	CreatedAt       time.Time
	InputTokens     *int `json:",omitempty"`
	Estimated       bool
	Outcome         string         `json:",omitempty"`
	Decision        string         `json:",omitempty"`
	Trigger         string         `json:",omitempty"`
	DefinitionID    string         `json:",omitempty"`
	WorkerSessionID atom.SessionID `json:",omitempty"`
	DurationMS      int64          `json:",omitempty"`
	Error           string         `json:",omitempty"`
	InputPreview    string         `json:",omitempty"`
	InputTruncated  bool           `json:",omitempty"`
	ContinueChat    *bool          `json:",omitempty"`
}

type PluginActivityPage struct {
	Name            string
	SessionID       atom.SessionID
	Revision        int64
	HistoryRevision int64
	Entries         []PluginActivityEntry
	NextCursor      string `json:",omitempty"`
}

// SessionActivityPlugin optionally exposes durable, read-only observability.
// Reading activity must not start workers, modify history, or commit reminders.
// Plugins own retention, pagination, and history-mutation fences.
type SessionActivityPlugin interface {
	SessionActivity(context.Context, atom.Session, PluginActivityQuery) (PluginActivityPage, error)
}
