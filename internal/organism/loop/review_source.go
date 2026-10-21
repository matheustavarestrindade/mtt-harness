package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

func (workers *sessionWorkers) Source(operationContext context.Context, sessionID atom.SessionID, maxBytes int, includeReasoning bool) (source harness.ReviewSource, operationError error) {
	if maxBytes < 1024 || maxBytes > 4<<20 {
		return source, fmt.Errorf("review source byte limit must be 1024-4194304")
	}
	source.Session, operationError = workers.loop.configuration.Store.Sessions().Get(operationContext, sessionID)
	if operationError != nil {
		return source, operationError
	}
	if source.Session.Execution != nil || source.Session.Parent != "" {
		return source, fmt.Errorf("review source must be a main session")
	}
	instance, found := workers.loop.configuration.Instances.Get(source.Session.InstanceID)
	if !found {
		return source, ErrInstanceStopped
	}
	source.Workspace = instance.Spec()
	source.Fence.Control, operationError = workers.queue.reviewControl(operationContext, source.Session)
	if operationError != nil {
		return source, operationError
	}
	if source.Fence.Control.PendingUser {
		return source, harness.ErrReviewStale
	}
	source.Tasks, operationError = workers.loop.configuration.Store.TaskStates().Get(operationContext, sessionID)
	if operationError != nil {
		return source, operationError
	}
	source.Fence.TaskRevision = source.Tasks.Revision
	source.Fence.HistoryRevision = source.Session.HistoryRevision
	source.Fence.UserMessageID, operationError = workers.loop.configuration.Store.Sessions().LatestUserMessageID(operationContext, sessionID)
	if operationError != nil {
		return source, operationError
	}
	var before int64
	remaining := maxBytes
	metadata, operationError := workers.loop.configuration.Store.Sessions().History(operationContext, sessionID, store.HistoryQuery{MetadataOnly: true, Revision: &source.Fence.HistoryRevision})
	if operationError != nil {
		return source, operationError
	}
	source.HeadSequence = metadata.HeadSeq
	var liveMessage *atom.Message
	active, live := workers.loop.LiveState(sessionID)
	source.Active = active
	if live != nil && (metadata.LatestResponse == nil || metadata.LatestResponse.MessageID != live.Message.ID) {
		source.LiveMessageID = live.Message.ID
		if len(live.Message.ToolCalls) > 0 || (includeReasoning && live.Message.Reasoning != "") || slices.ContainsFunc(live.Message.Content, func(content atom.Content) bool { return content.Text != "" || content.Type != atom.Text }) {
			message, omitted := boundedReviewMessage(live.Message, &remaining, includeReasoning)
			liveMessage = &message
			source.Truncated = omitted || live.Truncated
		}
	}
	for len(source.Messages) < 1024 && remaining > 512 {
		page, pageError := workers.loop.configuration.Store.Sessions().History(operationContext, sessionID, store.HistoryQuery{Before: before, Limit: min(100, max(1, remaining/512)), Revision: &source.Fence.HistoryRevision})
		if pageError != nil {
			return source, pageError
		}
		if before == 0 {
			source.HeadSequence = page.HeadSeq
		}
		for index := len(page.Messages) - 1; index >= 0; index-- {
			message, omitted := boundedReviewMessage(page.Messages[index], &remaining, includeReasoning)
			source.Truncated = source.Truncated || omitted
			source.Messages = append(source.Messages, message)
			if remaining <= 0 || len(source.Messages) >= 1024 {
				source.Truncated = true
				break
			}
		}
		if !page.HasOlder || len(page.Messages) == 0 {
			break
		}
		before = page.FirstSeq
		source.Truncated = source.Truncated || remaining <= 512 || len(source.Messages) >= 1024
	}
	slices.Reverse(source.Messages)
	if liveMessage != nil && !slices.ContainsFunc(source.Messages, func(message atom.Message) bool { return message.ID == liveMessage.ID }) {
		source.Messages = append(source.Messages, *liveMessage)
	}
	current, operationError := workers.Current(operationContext, source)
	if operationError != nil {
		return source, operationError
	}
	if !current {
		return source, harness.ErrReviewStale
	}
	return source, nil
}

func boundedReviewMessage(message atom.Message, remaining *int, includeReasoning bool) (atom.Message, bool) {
	result := message
	result.ProviderState = nil
	result.Usage = nil
	result.Content = nil
	result.ToolCalls = nil
	omitted := false
	copyText := func(text string) string {
		limit := min(len(text), max(0, *remaining))
		if limit < len(text) {
			omitted = true
			for limit > 0 && !utf8.RuneStart(text[limit]) {
				limit--
			}
		}
		*remaining -= limit
		return strings.Clone(text[:limit])
	}
	copyTail := func(text string) string {
		limit := min(len(text), max(0, *remaining))
		start := len(text) - limit
		if start > 0 {
			omitted = true
			for start < len(text) && !utf8.RuneStart(text[start]) {
				start++
			}
		}
		*remaining -= len(text) - start
		return strings.Clone(text[start:])
	}
	*remaining -= min(*remaining, 256)
	for _, content := range message.Content {
		if *remaining <= 0 {
			omitted = true
			break
		}
		entry := atom.Content{Type: content.Type, Filename: copyText(content.Filename), MIME: copyText(content.MIME)}
		if content.Type == atom.Text {
			entry.Text = copyTail(content.Text)
		}
		if len(content.Data) > 0 || content.URL != "" || content.AudioID != "" {
			omitted = true
		}
		result.Content = append(result.Content, entry)
	}
	result.Reasoning = ""
	if includeReasoning {
		result.Reasoning = copyTail(message.Reasoning)
	}
	for _, call := range message.ToolCalls {
		if *remaining <= 0 {
			omitted = true
			break
		}
		copyCall := call
		if len(call.Input) > min(*remaining, 65536) {
			copyCall.Input, _ = json.Marshal(map[string]any{"omitted_bytes": len(call.Input)})
			omitted = true
		} else {
			copyCall.Input = slices.Clone(call.Input)
		}
		*remaining -= len(copyCall.Input)
		result.ToolCalls = append(result.ToolCalls, copyCall)
	}
	return result, omitted
}

func (workers *sessionWorkers) Current(operationContext context.Context, source harness.ReviewSource) (bool, error) {
	session, operationError := workers.loop.configuration.Store.Sessions().Get(operationContext, source.Session.ID)
	if operationError != nil {
		return false, operationError
	}
	if session.InstanceID != source.Workspace.ID || session.HistoryRevision != source.Fence.HistoryRevision || session.Execution != nil {
		return false, nil
	}
	if _, found := workers.loop.configuration.Instances.Get(session.InstanceID); !found {
		return false, nil
	}
	control, operationError := workers.queue.reviewControl(operationContext, session)
	if operationError != nil {
		return false, operationError
	}
	if control.Epoch != source.Fence.Control.Epoch || control.Revision != source.Fence.Control.Revision || control.PendingUser {
		return false, nil
	}
	state, operationError := workers.loop.configuration.Store.TaskStates().Get(operationContext, session.ID)
	if operationError != nil {
		return false, operationError
	}
	if state.Revision != source.Fence.TaskRevision {
		return false, nil
	}
	userID, operationError := workers.loop.configuration.Store.Sessions().LatestUserMessageID(operationContext, session.ID)
	return userID == source.Fence.UserMessageID, operationError
}
func (workers *sessionWorkers) Observe(operationContext context.Context, session atom.Session) (<-chan struct{}, func(), error) {
	if operationError := operationContext.Err(); operationError != nil {
		return nil, nil, operationError
	}
	updates, release := workers.loop.SubscribeSessionState(session)
	return updates, release, nil
}

func (workers *sessionWorkers) Permission(operationContext context.Context, sessionID atom.SessionID, identifier string) (*atom.PermissionRequest, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	if workers.loop.configuration.Broker == nil {
		return nil, nil
	}
	for _, request := range workers.loop.configuration.Broker.SessionRequests(sessionID) {
		if request.ID == identifier {
			if request.ToolCall != nil {
				call := *request.ToolCall
				call.Input = slices.Clone(call.Input)
				request.ToolCall = &call
			}
			return &request, nil
		}
	}
	return nil, nil
}
func (workers *sessionWorkers) ResolvePermission(operationContext context.Context, source harness.ReviewSource, requestID string, allow bool, actor, correction string) (bool, error) {
	if len(correction) > 65536 {
		return false, fmt.Errorf("permission correction exceeds 65536 bytes")
	}
	current, operationError := workers.Current(operationContext, source)
	if operationError != nil {
		return false, operationError
	}
	if !current {
		return false, harness.ErrReviewStale
	}
	kind := atom.VerdictDeny
	if allow {
		kind = atom.VerdictAllow
	}
	decision := atom.PermissionDecision{RequestID: requestID, SessionID: source.Session.ID, InstanceID: source.Session.InstanceID, Kind: kind, Scope: atom.ScopeOnce, Actor: actor}
	if !allow && correction != "" {
		return workers.queue.submitPermissionCorrection(operationContext, source.Session, source.Fence.Control, decision, correction)
	}
	return workers.queue.resolveReviewPermission(operationContext, source.Session, source.Fence.Control, decision)
}
func (workers *sessionWorkers) SubmitContinuation(operationContext context.Context, source harness.ReviewSource, identifier, text string) (bool, error) {
	if identifier == "" || strings.TrimSpace(text) == "" || len(text) > 65536 {
		return false, fmt.Errorf("runtime continuation requires an ID and 1-65536 text bytes")
	}
	current, operationError := workers.Current(operationContext, source)
	if operationError != nil {
		return false, operationError
	}
	if !current {
		return false, harness.ErrReviewStale
	}
	return workers.queue.submitReviewContinuation(operationContext, source.Session, source.Fence.Control, identifier, text)
}
