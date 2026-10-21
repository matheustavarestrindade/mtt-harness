package atom

import (
	"encoding/json"
	"slices"
)

// ToolGrant is an explicit capability. Operation-aware tools require a non-empty
// operation selection; tools without operations use the whole named capability.
type ToolGrant struct {
	Name       string   `json:"name"`
	Operations []string `json:"operations,omitempty"`
}

// ExecutionProfile is immutable, server-created session policy. It is persisted
// for lifecycle recovery, never accepted from a public session request or exposed
// in session JSON. Descendants inherit it and cannot widen their capabilities.
type ExecutionProfile struct {
	Kind            string      `json:"kind"`
	Agent           string      `json:"agent"`
	RunID           string      `json:"run_id"`
	SourceSessionID SessionID   `json:"source_session_id"`
	Tools           []ToolGrant `json:"tools"`
	SystemPrompt    string      `json:"system_prompt"`
	DecisionRoot    bool        `json:"decision_root"`
	MaxRounds       int         `json:"max_rounds"`
	MaxOutputTokens int         `json:"max_output_tokens"`
}

func (profile *ExecutionProfile) Clone() *ExecutionProfile {
	if profile == nil {
		return nil
	}
	copyProfile := *profile
	copyProfile.Tools = slices.Clone(profile.Tools)
	for index := range copyProfile.Tools {
		copyProfile.Tools[index].Operations = slices.Clone(copyProfile.Tools[index].Operations)
	}
	return &copyProfile
}

func (profile *ExecutionProfile) Grant(name string) (ToolGrant, bool) {
	if profile == nil {
		return ToolGrant{}, false
	}
	if name == "search_tool" || (profile.DecisionRoot && (name == "continue" || name == "interfere")) || (!profile.DecisionRoot && name == "finish") {
		return ToolGrant{Name: name}, true
	}
	for _, grant := range profile.Tools {
		if grant.Name == name {
			return grant, true
		}
	}
	return ToolGrant{}, false
}

func (session Session) IsWatcher() bool {
	return session.Execution != nil && session.Execution.Kind == "watcher"
}

// Public session metadata identifies the execution kind without revealing the
// authorization profile, its frozen prompt, or internal accounting references.
func (session Session) MarshalJSON() ([]byte, error) {
	type fields Session
	kind := ""
	if session.Execution != nil {
		kind = session.Execution.Kind
	}
	return json.Marshal(struct {
		fields
		Kind string `json:",omitempty"`
	}{fields: fields(session), Kind: kind})
}
