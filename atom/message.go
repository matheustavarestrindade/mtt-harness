package atom

import "time"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type SessionID string

type Message struct {
	ID         string
	SessionID  SessionID
	Seq        int64
	Role       Role
	Content    []Content
	ToolCalls  []ToolCall
	ToolCallID string
	Usage      *Usage
	CreatedAt  time.Time
}
