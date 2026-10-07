// Package contextplugin implements workspace memory and cache-aware context
// selection using only public harness services. Its database namespace and
// background jobs are independent of the live conversation store.
package contextplugin

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

const pluginName = "context"
const settingsKey = "plugin.context"
const pluginVersion = "0.1.0"

type compression string

const (
	compressionLow    compression = "L"
	compressionMedium compression = "M"
	compressionHigh   compression = "H"
	compressionIdea   compression = "I"
)

type summaries struct {
	Low    string `json:"low"`
	Medium string `json:"medium"`
	High   string `json:"high"`
}

// source is an immutable public-message snapshot. Its ordinal is never reused,
// including after revert; provider continuation state is never serialized.
type source struct {
	ID            int64          `json:"id"`
	SessionID     atom.SessionID `json:"session_id"`
	MessageID     string         `json:"message_id"`
	Sequence      int64          `json:"sequence"`
	Role          atom.Role      `json:"role"`
	Message       atom.Message   `json:"message"`
	Text          string         `json:"text"`
	Deleted       bool           `json:"deleted"`
	Indexed       bool           `json:"indexed"`
	IndexedChunks int            `json:"indexed_chunks"`
	IndexModel    string         `json:"index_model"`
	CreatedAt     time.Time      `json:"created_at"`
}

type memoryRecord struct {
	ID              string    `json:"id"`
	Version         int       `json:"version"`
	Title           string    `json:"title"`
	Categories      []string  `json:"categories"`
	Text            summaries `json:"text"`
	Kind            string    `json:"kind"`
	Status          string    `json:"status"`
	SourceIDs       []int64   `json:"source_ids"`
	Children        []string  `json:"children,omitempty"`
	Important       bool      `json:"important"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	LastRetrievedAt time.Time `json:"last_retrieved_at"`
	Retrievals      int64     `json:"retrievals"`
}

type presentation struct {
	MemoryID string      `json:"memory_id"`
	Version  int         `json:"version"`
	Level    compression `json:"level"`
	Text     string      `json:"text"`
}

// sessionView is the committed projection. Available describes the last request
// delivered to the model; source IDs outside it cannot be queued by a tool.
type sessionView struct {
	SessionID     atom.SessionID   `json:"session_id"`
	Epoch         int64            `json:"epoch"`
	Revision      int64            `json:"revision"`
	Initialized   bool             `json:"initialized"`
	Removed       map[string]bool  `json:"removed"`
	Sources       map[string]int64 `json:"sources"`
	Available     []int64          `json:"available"`
	Groups        [][]int64        `json:"groups"`
	Protected     map[int64]bool   `json:"protected"`
	Snapshot      []presentation   `json:"snapshot"`
	Promote       map[string]bool  `json:"promote"`
	WrapRequested bool             `json:"wrap_requested"`
	NoticeLevel   int              `json:"notice_level"`
	InputTokens   int              `json:"input_tokens"`
	InputLimit    int              `json:"input_limit"`
	Mutation      *historyMutation `json:"mutation,omitempty"`
	Deleted       bool             `json:"deleted"`
}

type historyMutation struct {
	ID            string  `json:"id"`
	Owner         string  `json:"owner"`
	Kind          string  `json:"kind"`
	KeepMessageID string  `json:"keep_message_id"`
	SourceIDs     []int64 `json:"source_ids"`
}

type memoryJob struct {
	ID                string           `json:"id"`
	SessionID         atom.SessionID   `json:"session_id"`
	Epoch             int64            `json:"epoch"`
	Kind              string           `json:"kind"`
	Agent             string           `json:"agent"`
	Status            string           `json:"status"`
	Priority          int              `json:"priority"`
	SourceIDs         []int64          `json:"source_ids"`
	ContextIDs        []int64          `json:"context_ids"`
	Remember          bool             `json:"remember"`
	Text              string           `json:"text"`
	Categories        []string         `json:"categories"`
	MemoryIDs         []string         `json:"memory_ids"`
	ResultIDs         []string         `json:"result_ids"`
	Attempts          int              `json:"attempts"`
	Failures          int              `json:"failures"`
	Page              int              `json:"page"`
	PlanHash          string           `json:"plan_hash"`
	Progress          int64            `json:"progress"`
	Proposals         []memoryProposal `json:"proposals,omitempty"`
	CandidateVersions map[string]int   `json:"candidate_versions,omitempty"`
	Lease             string           `json:"lease"`
	LeaseUntil        time.Time        `json:"lease_until"`
	NextAttempt       time.Time        `json:"next_attempt"`
	CreatedAt         time.Time        `json:"created_at"`
	Error             string           `json:"error,omitempty"`
}

type vectorChunk struct {
	Text   string
	Vector []float64
}

type searchQuery struct {
	Text           string
	Vector         []float64
	EmbeddingModel string
	Kinds          []string
	Categories     []string
	IncludeDeleted bool
	IncludeHistory bool
	Limit          int
}

type searchHit struct {
	Kind    string
	ID      string
	Version int
	Text    string
	Score   float64
}

type jobLocation struct {
	WorkspaceID string
	JobID       string
}

func newIdentifier() string {
	var bytes [16]byte
	// crypto/rand.Read fills the buffer or terminates on an unrecoverable system
	// RNG failure in supported Go versions; no weak identifier fallback exists.
	rand.Read(bytes[:])
	return hex.EncodeToString(bytes[:])
}

func encodeDocument(value any) (json.RawMessage, error) { return json.Marshal(value) }

func newSessionView(identifier atom.SessionID) sessionView {
	return sessionView{SessionID: identifier, Removed: map[string]bool{}, Sources: map[string]int64{}, Protected: map[int64]bool{}, Promote: map[string]bool{}}
}
