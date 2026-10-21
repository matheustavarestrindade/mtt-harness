package watcher

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

// User feedback is a durable, anchored reference, not queued work. This lets the
// next user reply reach the main model with the question that prompted it.
func (plugin *Plugin) saveUserFeedback(operationContext context.Context, source harness.ReviewSource, runs []runRecord, corrections []map[string]string, waitForUserReply bool) error {
	if len(source.Messages) == 0 {
		return fmt.Errorf("watcher user feedback has no message anchor")
	}
	identifiers := make([]string, 0, len(runs))
	for _, run := range runs {
		identifiers = append(identifiers, run.ID)
	}
	data, operationError := json.Marshal(map[string]any{"type": "watcher.intervention", "run_ids": identifiers, "continue_chat": false, "audience": "user", "notes": corrections})
	if operationError != nil {
		return operationError
	}
	text := "[watcher]\n" + string(data)
	if len(text) > 65536 {
		return fmt.Errorf("watcher user feedback exceeds 65536 bytes")
	}
	notice := reviewNotice{ID: newID(), AnchorID: source.Messages[len(source.Messages)-1].ID, Text: text}
	return plugin.database.UpdateSession(operationContext, source.Workspace.ID, string(source.Session.ID), func(state *sessionState) error {
		if state.Deleted || state.UserID != source.Fence.UserMessageID {
			return harness.ErrReviewStale
		}
		if len(state.Notices) >= 32 {
			return fmt.Errorf("watcher retained user-feedback limit reached")
		}
		state.Notices = append(state.Notices, notice)
		if waitForUserReply {
			state.WaitingForUser = true
			state.WaitingUserID = source.Fence.UserMessageID
		}
		return nil
	})
}

// Projection is pure against the request lease. Context fitting measures these
// immutable bytes and can exclude them only with their source anchor.
func (plugin *Plugin) ProjectReminders(operationContext context.Context, session atom.Session, request atom.Request) (atom.Request, error) {
	frozen, found := operationContext.Value(requestKey{plugin}).(requestConfiguration)
	if !found || frozen.state.Deleted || len(frozen.state.Notices) == 0 {
		return request, operationContext.Err()
	}
	present := map[string]bool{}
	for _, message := range request.Messages {
		present[message.ID] = true
	}
	anchored := map[string][]reviewNotice{}
	for _, notice := range frozen.state.Notices {
		if !present[pluginName+":feedback:"+notice.ID] {
			anchored[notice.AnchorID] = append(anchored[notice.AnchorID], notice)
		}
	}
	messages := make([]atom.Message, 0, len(request.Messages)+len(frozen.state.Notices))
	for _, message := range request.Messages {
		messages = append(messages, message)
		if message.Ephemeral {
			continue
		}
		for _, notice := range anchored[message.ID] {
			messages = append(messages, atom.Message{ID: pluginName + ":feedback:" + notice.ID, SessionID: session.ID, Role: atom.RoleRuntime, Ephemeral: true, Content: []atom.Content{{Type: atom.Text, Text: notice.Text}}})
		}
	}
	request.Messages = messages
	return request, operationContext.Err()
}
