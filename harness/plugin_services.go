package harness

import (
	"context"
	"errors"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

// PluginSettings uses the existing database setting owner. Empty workspace IDs
// select harness defaults. Plugin keys must be namespaced by plugin name.
var ErrConversationUnavailable = errors.New("conversation is not available")

type PluginSettings interface {
	Save(context.Context, string, string, string) error
	Get(context.Context, string, string) (string, error)
	Delete(context.Context, string, string) error
}

// ConversationReader exposes stored public messages without write access or
// private provider continuation fields. Missing/deleted sessions return errors.
type ConversationReader interface {
	Get(context.Context, atom.SessionID) (atom.Session, error)
	Messages(context.Context, atom.SessionID) ([]atom.Message, error)
}

// WorkspaceReader supplies runtime availability and the current model policy.
type WorkspaceReader interface {
	Workspace(context.Context, string) (atom.InstanceSpec, error)
}

// WorkspaceAgentRequest is a background model request, not a child session.
// RequestID identifies a single billed attempt; retries require new IDs.
type WorkspaceAgentRequest struct {
	WorkspaceID     string
	Agent           string
	RunID           string
	RequestID       string
	SourceSessionID atom.SessionID
	Model           string
	ReasoningEffort string
	Messages        []atom.Message
	MaxOutputTokens int
}

type WorkspaceAgentResponse struct {
	Model string
	Text  string
	Usage atom.Usage
}

// WorkspaceModels reuses provider routing, credentials, allowlists, effort
// validation and accounting. Background requests do not invoke conversation
// middleware or execute tools, preventing recursive plugin work.
type WorkspaceModels interface {
	Model(context.Context, string, string, string) (atom.ModelInfo, error)
	Run(context.Context, WorkspaceAgentRequest) (WorkspaceAgentResponse, error)
}

// TextEmbedder separates tokenizer-aware chunking from inference. Every byte
// must survive Split, and Embed encodes document chunks without truncation.
type TextEmbedder interface {
	Split(context.Context, string) ([]string, error)
	Embed(context.Context, []string) ([][]float64, error)
}

// QueryTextEmbedder is an optional asymmetric retrieval capability. Split must
// reserve enough tokens for both query and document preprocessing. Symmetric
// encoders may implement only TextEmbedder; callers then use Embed for queries.
type QueryTextEmbedder interface {
	EmbedQueries(context.Context, []string) ([][]float64, error)
}

// PromptRenderer renders startup-compatible, preloaded templates. Rendering
// never discovers tools, activates schemas, or reads a prompt file at runtime.
type PromptRenderer interface {
	RenderPrompt(context.Context, atom.Session, string, string) (string, error)
}

var ErrWorkspaceMemoryUnavailable = errors.New("workspace memory is unavailable")

// MemoryQuery is scoped retrieval for a workspace worker. Attribution affects
// counters only; SourceSessionID does not become an owning billing session.
type MemoryQuery struct {
	WorkspaceID     string
	SourceSessionID atom.SessionID
	Agent           string
	RunID           string
	Query           string
	Categories      []string
	Compression     string
	Limit           int
	MaxBytes        int
}

// MemoryReference is current, non-deleted workspace data. Reference identifiers
// are worker provenance, not text to add to user-facing replies.
type MemoryReference struct {
	ID         string
	Version    int
	Categories []string
	Text       string
}

// WorkspaceMemory performs read-only retrieval without changing a session's
// frozen memory snapshot or promotion levels. It never exposes archived secrets.
type WorkspaceMemory interface {
	SearchWorkspaceMemory(context.Context, MemoryQuery) ([]MemoryReference, error)
}

// PluginServices are explicit application dependencies. Plugin implementations
// never need an import of a harness internal package.
type PluginServices struct {
	Settings      PluginSettings
	Conversations ConversationReader
	Workspaces    WorkspaceReader
	Models        WorkspaceModels
	Embeddings    TextEmbedder
	Usage         WorkspaceUsage
	Prompts       PromptRenderer
	Memory        WorkspaceMemory
}

type WorkspaceUsage interface {
	Agents(context.Context, string) ([]atom.AgentStatistics, error)
}
