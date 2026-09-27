package atom

import "time"

type NotifyMode string

const (
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
