package atom

// AgentOrigin links a child to the recorded spawning response. Legacy origins
// use ordered model.call/agent.start events, never task-text or timestamp guesses.
type AgentOrigin struct {
	SessionID  SessionID
	Parent     SessionID
	MessageID  string
	ToolCallID string
}
