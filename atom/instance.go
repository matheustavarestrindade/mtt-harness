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
}

type Session struct {
	ID         SessionID
	InstanceID string
	Parent     SessionID
	Depth      int
	Model      string
	CreatedAt  time.Time
}

type AgentTask struct {
	Task  string
	Model string
}
