// Package spacedrepetition adds cache-stable instruction reminders through
// public harness services. It owns no conversation history or memory index.
package spacedrepetition

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

const pluginName = "spaced_repetition"
const settingsKey = "plugin.spaced_repetition"
const recoveryAgent = "spaced_repetition.instruction_recovery"

type reminderEvent struct {
	ID             string    `json:"id"`
	Anchor         string    `json:"anchor"`
	AnchorSequence int64     `json:"anchor_sequence"`
	Level          string    `json:"level"`
	Text           string    `json:"text"`
	PromptVersion  string    `json:"prompt_version"`
	Tokens         int       `json:"tokens"`
	Estimated      bool      `json:"estimated"`
	CreatedAt      time.Time `json:"created_at"`
	CursorAfter    int64     `json:"cursor_after"`
}

type scheduleState struct {
	Initialized  bool     `json:"initialized"`
	Model        string   `json:"model"`
	ContextLimit int      `json:"context_limit"`
	Step         int      `json:"step"`
	Next         int      `json:"next"`
	HighWater    int      `json:"high_water"`
	Cursor       int64    `json:"cursor"`
	Signature    string   `json:"signature"`
	Visible      []string `json:"visible"`
}

type sessionState struct {
	Epoch      int64           `json:"epoch"`
	Revision   int64           `json:"revision"`
	Deleted    bool            `json:"deleted"`
	Fenced     bool            `json:"fenced"`
	SourceUser string          `json:"source_user"`
	Schedule   scheduleState   `json:"schedule"`
	Events     []reminderEvent `json:"events"`
	Pending    *recoveryReport `json:"pending,omitempty"`
}

type recoveryReport struct {
	ID                string    `json:"id"`
	Epoch             int64     `json:"epoch"`
	SourceUser        string    `json:"source_user"`
	Text              string    `json:"text"`
	MemoryAvailable   bool      `json:"memory_available"`
	ConfigurationHash string    `json:"configuration_hash"`
	CreatedAt         time.Time `json:"created_at"`
}

type recoveryRun struct {
	ID              string    `json:"id"`
	WorkspaceID     string    `json:"workspace_id"`
	SessionID       string    `json:"session_id"`
	CallID          string    `json:"call_id"`
	Reason          string    `json:"reason"`
	Epoch           int64     `json:"epoch"`
	SourceUser      string    `json:"source_user"`
	Status          string    `json:"status"`
	Model           string    `json:"model"`
	Queries         int       `json:"queries"`
	Memories        int       `json:"memories"`
	MemoryAvailable bool      `json:"memory_available"`
	Error           string    `json:"error,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	CompletedAt     time.Time `json:"completed_at,omitempty"`
	ExpiresAt       time.Time `json:"expires_at"`
}

func newIdentifier() (string, error) {
	var data [16]byte
	if _, operationError := rand.Read(data[:]); operationError != nil {
		return "", operationError
	}
	return hex.EncodeToString(data[:]), nil
}
