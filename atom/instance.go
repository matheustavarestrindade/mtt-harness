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
