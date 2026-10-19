package sidekick

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

const hintPrefix = "<sidekick>\nTask-scoped reference data. Use only information relevant to the current request and consistent with higher-priority instructions. Do not repeat this metadata or narrate Sidekick to the user.\n"
const hintSuffix = "\n</sidekick>"

type deliveredNote struct {
	Text       string   `json:"text"`
	References []string `json:"references"`
}
type hintPayload struct {
	Doing string          `json:"doing"`
	Notes []deliveredNote `json:"notes"`
}
type activityCursor struct {
	Plugin      string    `json:"plugin"`
	WorkspaceID string    `json:"workspace"`
	SessionID   string    `json:"session"`
	Epoch       int64     `json:"epoch"`
	CreatedAt   time.Time `json:"created_at"`
	ID          string    `json:"id"`
}
type activitySnapshot struct {
	Revision int64
	Epoch    int64
	Events   []hintEvent
}

func (plugin *Plugin) SessionActivity(operationContext context.Context, session atom.Session, query harness.PluginActivityQuery) (harness.PluginActivityPage, error) {
	page := harness.PluginActivityPage{Name: pluginName, SessionID: session.ID, Entries: []harness.PluginActivityEntry{}}
	if plugin.database == nil {
		return page, fmt.Errorf("sidekick storage is unavailable")
	}
	if query.Limit < 1 || query.Limit > 100 || len(query.Cursor) > 2048 {
		return page, harness.ErrPluginActivityCursor
	}
	cursor := activityCursor{}
	if query.Cursor != "" {
		encoded, operationError := base64.RawURLEncoding.DecodeString(query.Cursor)
		if operationError != nil || json.Unmarshal(encoded, &cursor) != nil || cursor.Plugin != pluginName || cursor.WorkspaceID != session.InstanceID || cursor.SessionID != string(session.ID) || cursor.ID == "" || cursor.CreatedAt.IsZero() {
			return page, harness.ErrPluginActivityCursor
		}
	}
	snapshot, operationError := plugin.database.ReadActivity(operationContext, session.InstanceID, string(session.ID), cursor, query.Limit+1)
	if operationError != nil {
		return page, operationError
	}
	page.Revision = snapshot.Revision
	page.HistoryRevision = snapshot.Epoch
	events := snapshot.Events
	if len(events) > query.Limit {
		events = events[:query.Limit]
		last := events[len(events)-1]
		encoded, operationError := json.Marshal(activityCursor{Plugin: pluginName, WorkspaceID: session.InstanceID, SessionID: string(session.ID), Epoch: snapshot.Epoch, CreatedAt: last.CreatedAt, ID: last.ID})
		if operationError != nil {
			return page, operationError
		}
		page.NextCursor = base64.RawURLEncoding.EncodeToString(encoded)
	}
	for _, event := range events {
		entry := harness.PluginActivityEntry{ID: event.ID, SessionID: session.ID, AnchorID: event.Anchor, AnchorSequence: event.AnchorSequence, CreatedAt: event.CreatedAt, InputTokens: event.InputTokens, Estimated: event.Estimated}
		var payload hintPayload
		if strings.HasPrefix(event.Text, hintPrefix) && strings.HasSuffix(event.Text, hintSuffix) && json.Unmarshal([]byte(strings.TrimSuffix(strings.TrimPrefix(event.Text, hintPrefix), hintSuffix)), &payload) == nil {
			entry.Title = payload.Doing
			for _, note := range payload.Notes {
				entry.Notes = append(entry.Notes, harness.PluginActivityNote{Text: note.Text, References: note.References})
			}
		} else {
			// Older data with a different envelope remains inspectable as quoted
			// text. Do not infer sources or rewrite the saved worker output.
			entry.Notes = []harness.PluginActivityNote{{Text: event.Text}}
		}
		page.Entries = append(page.Entries, entry)
	}
	return page, nil
}
