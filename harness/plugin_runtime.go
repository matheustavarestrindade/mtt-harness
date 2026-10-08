package harness

import (
	"context"
	"encoding/json"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

// RequestBudget measures the provider-bound request after model selection. A
// zero InputLimit means the provider has not supplied a context limit.
type RequestBudget struct {
	ContextLimit  int
	OutputReserve int
	InputLimit    int
	InputTokens   int
	Estimated     bool
}

// ContextRequest is evaluated between completed tool groups. Measure uses the
// same counter and reservations as the core fitter; it performs no model call.
type ContextRequest struct {
	Session atom.Session
	Request atom.Request
	Model   atom.ModelInfo
	// ModelID is the resolved provider-qualified selection after middleware.
	ModelID string
	Budget  RequestBudget
	Measure func(context.Context, atom.Request) (RequestBudget, error)
	// InputCeiling is an optional stricter context-policy limit, including all
	// projected reminders. Zero uses the model input limit.
	InputCeiling int
}

// ContextSelection contains a request-local view. Managed disables automatic
// history trimming, but never disables the final core input-budget check.
type ContextSelection struct {
	Request      atom.Request
	Managed      bool
	InputCeiling int
}

// RequestLifecyclePlugin freezes request-local plugin configuration before
// tool discovery. Release runs after streaming and the entire tool group end.
// A failed BeginRequest must not return a live lease. Release must not panic.
type RequestLifecyclePlugin interface {
	BeginRequest(context.Context, atom.Session) (context.Context, func(), error)
}

// ContextPolicyPlugin selects the provider request without changing the model,
// reasoning effort, tool definitions, or provider parameters.
type ContextPolicyPlugin interface {
	PrepareContext(context.Context, ContextRequest) (ContextSelection, error)
}

type HistoryChangeKind string

const (
	HistoryRevert HistoryChangeKind = "revert"
	HistoryDelete HistoryChangeKind = "delete"
)

// HistorySnapshot contains stored, public conversation data before a mutation.
// Provider continuation state is deliberately excluded by the host adapter.
type HistorySnapshot struct {
	Session  atom.Session
	Messages []atom.Message
}

type HistoryChange struct {
	Kind          HistoryChangeKind
	WorkspaceID   string
	KeepMessageID string
	Snapshots     []HistorySnapshot
}

// HistoryLifecyclePlugin archives and fences its workers before core history
// changes. The completion callback receives whether storage committed. It must
// preserve enough durable information to recover if completion is interrupted.
type HistoryLifecyclePlugin interface {
	BeginHistoryChange(context.Context, HistoryChange) (func(context.Context, bool) error, error)
}

// PluginRuntime is the host's ordered dispatch surface. Callbacks run in
// workers, outside queue owners and registry locks.
type PluginRuntime interface {
	BeginRequest(context.Context, atom.Session) (context.Context, func(), error)
	PrepareContext(context.Context, ContextRequest) (ContextSelection, error)
	BeginHistoryChange(context.Context, HistoryChange) (func(context.Context, bool) error, error)
}

// PluginState separates requested settings from applied state during a live
// transition. Configuration contains no credentials.
type PluginState struct {
	Name             string
	Version          string
	WorkspaceID      string
	Available        bool
	Enabled          bool
	RequestedEnabled bool
	Pending          bool
	Configuration    json.RawMessage
	Override         json.RawMessage
	Schema           atom.Schema
	Error            string `json:",omitempty"`
}

// ConfigurablePlugin owns validation of its namespaced database settings.
// Updates are partial; null values remove workspace overrides.
type ConfigurablePlugin interface {
	ReadState(context.Context, string) (PluginState, error)
	UpdateConfiguration(context.Context, string, json.RawMessage) (PluginState, error)
}

// ClosingPlugin joins its workers before dependent providers and storage close.
type ClosingPlugin interface {
	Close(context.Context) error
}

// TurnLifecyclePlugin observes a joined turn. Status is completed, error, or
// cancelled. The callback runs in the turn worker, before admission is released.
type TurnLifecyclePlugin interface {
	EndTurn(context.Context, atom.Session, string) error
}

// PluginMetrics uses workspace-level accounting. It exposes no memory content
// and is independent of any chat session's statistics.
type PluginMetrics struct {
	Name        string
	WorkspaceID string
	Counters    map[string]int64
	Agents      []atom.AgentStatistics
}

type ObservablePlugin interface {
	Metrics(context.Context, string) (PluginMetrics, error)
}
