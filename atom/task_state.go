package atom

import "time"

// TaskStatus records work progress independently of session execution status.
type TaskStatus string

const (
	TaskPending    TaskStatus = "pending"
	TaskInProgress TaskStatus = "in_progress"
	TaskDone       TaskStatus = "done"
	TaskCancelled  TaskStatus = "cancelled"
)

// TaskItem retains its ID and order across partial progress updates.
type TaskItem struct {
	ID     string
	Title  string
	Status TaskStatus
}

// DoingState is a concise, user-visible account of the current work.
type DoingState struct {
	Title       string
	Description string
}

// TaskState is session-owned working state. ResponsesSinceUpdate is internal
// scheduling data; a response counter does not represent user-visible progress.
type TaskState struct {
	SessionID            SessionID
	Todo                 []TaskItem
	Doing                *DoingState
	Revision             int64
	UpdatedAt            time.Time
	ResponsesSinceUpdate int `json:"-"`
}

// TaskItemUpdate preserves fields omitted from an existing item's update.
type TaskItemUpdate struct {
	ID     string      `json:"id"`
	Title  *string     `json:"title,omitempty"`
	Status *TaskStatus `json:"status,omitempty"`
}

// A nil Todo leaves the list alone; an empty slice clears it. DoingProvided
// distinguishes an omitted doing field from an explicit null that clears it.
type TaskStateUpdate struct {
	Todo          *[]TaskItemUpdate
	Doing         *DoingState
	DoingProvided bool
}
