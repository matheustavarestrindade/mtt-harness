package atom

import (
	"encoding/json"
	"time"
)

type EventName string

const (
	EventSessionStart       EventName = "session.start"
	EventSessionEnd         EventName = "session.end"
	EventTurnStart          EventName = "turn.start"
	EventTurnEnd            EventName = "turn.end"
	EventModelCall          EventName = "model.call"
	EventModelChunk         EventName = "model.chunk"
	EventActionReceived     EventName = "action.received"
	EventToolStart          EventName = "tool.start"
	EventToolEnd            EventName = "tool.end"
	EventPermissionRequest  EventName = "permission.request"
	EventPermissionDecision EventName = "permission.decision"
	EventProcessStart       EventName = "process.start"
	EventProcessOutput      EventName = "process.output"
	EventProcessExit        EventName = "process.exit"
	EventProcessNotify      EventName = "process.notification"
	EventAgentStart         EventName = "agent.start"
	EventAgentEnd           EventName = "agent.end"
	EventTaskStateUpdated   EventName = "task_state.updated"
)

type Event struct {
	Seq        uint64
	InstanceID string
	SessionID  SessionID
	Name       EventName
	Payload    json.RawMessage
	Time       time.Time
}
