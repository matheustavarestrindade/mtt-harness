package atom

import "time"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	// RoleRuntime contains background process data, not a user request or system instruction.
	RoleRuntime Role = "runtime"
)

type SessionID string

type Message struct {
	ID        string
	SessionID SessionID
	Seq       int64
	Role      Role
	Content   []Content
	// Reasoning is provider-exposed thinking text or a reasoning summary for display.
	Reasoning     string `json:",omitempty"`
	ToolCalls     []ToolCall
	ToolCallID    string
	Usage         *Usage
	ProviderState *ProviderState `json:"-"`
	CreatedAt     time.Time
	// Ephemeral marks request-local data. It is not a persisted input turn and
	// must not create a new trimming boundary or enter a conversation archive.
	Ephemeral bool `json:"-"`
	// InContext keeps a trusted request-local system reminder at its conversation
	// position. Provider adapters must not fold it into startup instructions.
	InContext bool `json:"-"`
}
