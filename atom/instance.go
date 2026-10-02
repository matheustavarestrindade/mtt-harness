package atom

import "time"

type InstanceSpec struct {
	ID              string
	Workspace       string
	Models          []string
	DefaultModel    string
	ProcessLimit    int
	AgentDepthLimit int
	CreatedAt       time.Time
	Stopped         bool
}

type Session struct {
	ID              SessionID
	InstanceID      string
	Parent          SessionID
	Depth           int
	Model           string
	ReasoningEffort string `json:",omitempty"`
	CreatedAt       time.Time
	Completed       bool
}

type AgentTask struct {
	Task            string
	Model           string
	ReasoningEffort string
}

// SessionModelSelection is one atomic configuration snapshot. Model and effort
// must change together so a model cannot receive another model's effort choice.
type SessionModelSelection struct {
	Model           string
	ReasoningEffort string
}

func (session Session) ModelSelection() SessionModelSelection {
	return SessionModelSelection{Model: session.Model, ReasoningEffort: session.ReasoningEffort}
}
