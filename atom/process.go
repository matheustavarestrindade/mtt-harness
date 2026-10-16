package atom

import "time"

type NotifyMode string

const (
	NotifyNone     NotifyMode = "none"
	NotifyExit     NotifyMode = "exit"
	NotifyError    NotifyMode = "error"
	NotifyInterval NotifyMode = "interval"
)

type NotifyPolicy struct {
	Mode     NotifyMode
	Interval time.Duration
}

type ProcessSpec struct {
	Command string
	Args    []string
	Cwd     string
	Env     []string
	Notify  NotifyPolicy
	Timeout time.Duration
	// Terminal attaches stdin/stdout/stderr to one PTY. Its bytes are reported on
	// stdout; pipe mode retains separate stdout/stderr streams.
	Terminal bool `json:",omitempty"`
	Columns  int  `json:",omitempty"`
	Rows     int  `json:",omitempty"`
	// IdleTimeout bounds a foreground quiet wait or one background inactivity
	// notice. Zero disables inactivity handling; it never stops the command.
	IdleTimeout time.Duration
	// Tool references are assigned by the runtime, never by shell arguments.
	ToolCallID    string `json:",omitempty"`
	ToolMessageID string `json:",omitempty"`
}

type StreamName string

const (
	StreamStart  StreamName = "start"
	StreamStdout StreamName = "stdout"
	StreamStderr StreamName = "stderr"
	StreamExit   StreamName = "exit"
)

type ProcessEvent struct {
	ProcessID string
	Stream    StreamName
	Data      []byte
	Error     string
	At        time.Time
	// Offset is a zero-based byte position in this stream after output middleware.
	// Snapshot marks retained output supplied when a subscriber joins.
	Offset   int64
	Snapshot bool
	Exit     *ExitStatus `json:",omitempty"`
}

type Signal string

type ExitStatus struct {
	Code   int
	Signal Signal
	Error  string
}

type ProcessRecord struct {
	ID         string
	InstanceID string
	SessionID  SessionID
	Spec       ProcessSpec
	PID        int
	Status     string
	Exit       *ExitStatus
	StartedAt  time.Time
	EndedAt    time.Time
}
