package loop

import (
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

const liveTextLimit = 100_000

type sessionStateListener struct {
	session atom.Session
	changed chan struct{}
}
type liveResponseState struct {
	message                              atom.Message
	model                                string
	revision                             uint64
	text, reasoning                      []string
	textBytes, reasoningBytes, toolBytes int
	truncated                            bool
}

// SubscribeSessionState subscribes to coalesced invalidations, not state values.
// Owners only signal metadata here. Snapshot reads, callbacks, and socket I/O
// run outside owners; a slow viewer never blocks execution or persistence.
func (agentLoop *Loop) SubscribeSessionState(session atom.Session) (<-chan struct{}, func()) {
	agentLoop.mutex.Lock()
	defer agentLoop.mutex.Unlock()
	if agentLoop.stateListeners == nil {
		agentLoop.stateListeners = map[uint64]sessionStateListener{}
	}
	agentLoop.nextStateListener++
	identifier := agentLoop.nextStateListener
	changed := make(chan struct{}, 1)
	agentLoop.stateListeners[identifier] = sessionStateListener{session, changed}
	return changed, sync.OnceFunc(func() { agentLoop.mutex.Lock(); delete(agentLoop.stateListeners, identifier); agentLoop.mutex.Unlock() })
}

func (agentLoop *Loop) NotifySessionState(sessionID atom.SessionID) {
	agentLoop.mutex.Lock()
	defer agentLoop.mutex.Unlock()
	agentLoop.notifySessionStateLocked(sessionID, "")
}
func (agentLoop *Loop) notifyWorkspaceState(instanceID string) {
	agentLoop.mutex.Lock()
	defer agentLoop.mutex.Unlock()
	agentLoop.notifySessionStateLocked("", instanceID)
}
func (agentLoop *Loop) notifySessionStateLocked(sessionID atom.SessionID, instanceID string) {
	for _, listener := range agentLoop.stateListeners {
		if listener.session.ID != sessionID && (instanceID == "" || listener.session.InstanceID != instanceID) {
			continue
		}
		select {
		case listener.changed <- struct{}{}:
		default:
		}
	}
}

func (agentLoop *Loop) LiveState(sessionID atom.SessionID) (bool, *atom.LiveResponse) {
	agentLoop.mutex.Lock()
	defer agentLoop.mutex.Unlock()
	active := agentLoop.activeRuns[sessionID]
	if active == nil {
		return false, nil
	}
	if active.response == nil {
		return true, nil
	}
	state := active.response
	message := state.message
	message.Content = []atom.Content{{Type: atom.Text, Text: strings.Join(state.text, "")}}
	message.Reasoning = strings.Join(state.reasoning, "")
	message.ToolCalls = slices.Clone(state.message.ToolCalls)
	return true, &atom.LiveResponse{Message: message, Model: state.model, Revision: state.revision, Truncated: state.truncated}
}

func (agentLoop *Loop) ActiveSessions(instanceID string) []atom.SessionID {
	agentLoop.mutex.Lock()
	defer agentLoop.mutex.Unlock()
	identifiers := []atom.SessionID{}
	for identifier, active := range agentLoop.activeRuns {
		if active.session.InstanceID == instanceID {
			identifiers = append(identifiers, identifier)
		}
	}
	return identifiers
}

func (agentLoop *Loop) beginLiveResponse(session atom.Session, messageID, model string) {
	agentLoop.mutex.Lock()
	defer agentLoop.mutex.Unlock()
	if active := agentLoop.activeRuns[session.ID]; active != nil {
		active.response = &liveResponseState{message: atom.Message{ID: messageID, SessionID: session.ID, Role: atom.RoleAssistant, CreatedAt: time.Now()}, model: model}
	}
}

func (agentLoop *Loop) appendLiveText(sessionID atom.SessionID, text string, reasoning bool) uint64 {
	agentLoop.mutex.Lock()
	defer agentLoop.mutex.Unlock()
	active := agentLoop.activeRuns[sessionID]
	if active == nil || active.response == nil {
		return 0
	}
	state := active.response
	if reasoning {
		state.reasoning, state.reasoningBytes = appendLiveTail(state.reasoning, state.reasoningBytes, text, &state.truncated)
	} else {
		state.text, state.textBytes = appendLiveTail(state.text, state.textBytes, text, &state.truncated)
	}
	state.revision++
	return state.revision
}

func appendLiveTail(chunks []string, size int, text string, truncated *bool) ([]string, int) {
	chunks = append(chunks, text)
	size += len(text)
	for len(chunks) > 1 && (size > liveTextLimit || len(chunks) > 512) {
		size -= len(chunks[0])
		chunks[0] = ""
		chunks = chunks[1:]
		*truncated = true
	}
	if size > liveTextLimit {
		text = chunks[0]
		start := len(text) - liveTextLimit
		for start < len(text) && !utf8.RuneStart(text[start]) {
			start++
		}
		chunks[0] = strings.Clone(text[start:])
		size = len(chunks[0])
		*truncated = true
	}
	return chunks, size
}

func (agentLoop *Loop) appendLiveTool(sessionID atom.SessionID, call atom.ToolCall) {
	agentLoop.mutex.Lock()
	defer agentLoop.mutex.Unlock()
	active := agentLoop.activeRuns[sessionID]
	if active == nil || active.response == nil {
		return
	}
	state := active.response
	if len(state.message.ToolCalls) >= 32 || state.toolBytes+len(call.Input) > 262144 {
		state.truncated = true
		return
	}
	call.Input = slices.Clone(call.Input)
	state.toolBytes += len(call.Input)
	state.message.ToolCalls = append(state.message.ToolCalls, call)
}

func (messageQueue *Queue) SubscribeSessionState(session atom.Session) (<-chan struct{}, func()) {
	return messageQueue.loop.SubscribeSessionState(session)
}
func (messageQueue *Queue) NotifySessionState(sessionID atom.SessionID) {
	messageQueue.loop.NotifySessionState(sessionID)
}
func (messageQueue *Queue) LiveState(sessionID atom.SessionID) (bool, *atom.LiveResponse) {
	return messageQueue.loop.LiveState(sessionID)
}

func (messageQueue *Queue) ActiveSessions(instanceID string) []atom.SessionID {
	return messageQueue.loop.ActiveSessions(instanceID)
}
