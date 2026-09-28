package atom

import "time"

type VerdictKind string

const (
	VerdictAllow VerdictKind = "allow"
	VerdictAsk   VerdictKind = "ask"
	VerdictDeny  VerdictKind = "deny"
)

type Verdict struct {
	Kind   VerdictKind
	Target string
	Why    string
}

type PermissionRequest struct {
	ID         string
	InstanceID string
	SessionID  SessionID
	Target     string
	Why        string
}

type Scope string

const (
	ScopeOnce    Scope = "once"
	ScopeSession Scope = "session"
	ScopeAlways  Scope = "always"
)

type PermissionDecision struct {
	RequestID  string
	InstanceID string
	SessionID  SessionID
	Target     string
	Kind       VerdictKind
	Scope      Scope
	CreatedAt  time.Time
}
