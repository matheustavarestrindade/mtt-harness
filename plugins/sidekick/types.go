package sidekick

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

const pluginName = "sidekick"
const settingsKey = "plugin.sidekick"
const agentName = "sidekick.context"

var errRetired = errors.New("sidekick source is no longer current")

type hintEvent struct {
	ID             string    `json:"id"`
	Anchor         string    `json:"anchor"`
	AnchorSequence int64     `json:"anchor_sequence"`
	Text           string    `json:"text"`
	Sources        []string  `json:"sources"`
	CreatedAt      time.Time `json:"created_at"`
}

type preparedHint struct {
	ID                string                           `json:"id"`
	Key               string                           `json:"key"`
	Epoch             int64                            `json:"epoch"`
	SourceUser        string                           `json:"source_user"`
	TaskKey           string                           `json:"task_key"`
	ConfigurationHash string                           `json:"configuration_hash"`
	Text              string                           `json:"text"`
	Sources           []string                         `json:"sources"`
	Files             []harness.WorkspaceFileReference `json:"files,omitempty"`
	Memories          []harness.MemoryReference        `json:"memories,omitempty"`
	CreatedAt         time.Time                        `json:"created_at"`
}

type sessionState struct {
	Revision          int64         `json:"revision"`
	Epoch             int64         `json:"epoch"`
	Deleted           bool          `json:"deleted"`
	Fenced            bool          `json:"fenced"`
	SourceUser        string        `json:"source_user"`
	TaskKey           string        `json:"task_key"`
	ConfigurationHash string        `json:"configuration_hash"`
	LastKey           string        `json:"last_key"`
	LastStatus        string        `json:"last_status"`
	LastStarted       time.Time     `json:"last_started"`
	Pending           *preparedHint `json:"pending,omitempty"`
	Events            []hintEvent   `json:"events,omitempty"`
}

type workerRun struct {
	ID          string         `json:"id"`
	WorkspaceID string         `json:"workspace_id"`
	SessionID   atom.SessionID `json:"session_id"`
	Key         string         `json:"key"`
	Epoch       int64          `json:"epoch"`
	Status      string         `json:"status"`
	Error       string         `json:"error,omitempty"`
	StartedAt   time.Time      `json:"started_at"`
	ExpiresAt   time.Time      `json:"expires_at"`
	CompletedAt time.Time      `json:"completed_at"`
	MemoryCount int            `json:"memory_count"`
	FileCount   int            `json:"file_count"`
}

type mutation struct {
	Counters  map[string]int64
	Run       *workerRun
	PurgeRuns bool
	RunID     string
	RunStatus string
}

type sourceData struct {
	Key    string                          `json:"key"`
	Origin string                          `json:"origin"`
	Text   string                          `json:"text"`
	File   *harness.WorkspaceFileReference `json:"-"`
	Memory *harness.MemoryReference        `json:"-"`
}

func doingFingerprint(doing *atom.DoingState) string {
	if doing == nil {
		return ""
	}
	return digestText(strings.Join(strings.Fields(doing.Title), " ") + "\n" + strings.Join(strings.Fields(doing.Description), " "))
}
func digestText(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
func configurationFingerprint(value configuration, promptVersion string) string {
	data, _ := json.Marshal(value)
	return digestText(promptVersion + string(data))
}
func newIdentifier() string { return rand.Text() }
