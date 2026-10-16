package loop

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

const queuedPreviewBytes = 4096
const queuedPreviewParts = 32

// StatusWithMessages obtains IDs and bounded previews from the same owner turn.
// No I/O or unbounded body copying runs in the session coordinator.
func (messageQueue *Queue) StatusWithMessages(operationContext context.Context, sessionID atom.SessionID) (QueueStatus, error) {
	coordinator, operationError := messageQueue.coordinator(operationContext, atom.Session{ID: sessionID}, false)
	if operationError != nil || coordinator == nil {
		return QueueStatus{}, operationError
	}
	response, operationError := coordinator.requestSessionCommand(operationContext, sessionCommand{kind: readStatus, includeQueuedContent: true})
	return response.status, operationError
}

func previewQueuedMessage(message atom.Message) atom.QueuedMessagePreview {
	preview := atom.QueuedMessagePreview{ID: message.ID, SessionID: message.SessionID, Role: message.Role, CreatedAt: message.CreatedAt, Content: []atom.Content{}}
	remainingBytes := queuedPreviewBytes
	copyPreviewText := func(text string) string {
		limit := min(len(text), remainingBytes)
		if limit < len(text) {
			preview.Truncated = true
			for limit > 0 && !utf8.RuneStart(text[limit]) {
				limit--
			}
		}
		remainingBytes -= limit
		return strings.Clone(text[:limit])
	}
	for index, content := range message.Content {
		if index >= queuedPreviewParts || remainingBytes < 128 {
			preview.Truncated = true
			break
		}
		remainingBytes -= 128 // Bound per-part JSON/metadata overhead as well as text.
		previewContent := atom.Content{Type: content.Type, MIME: copyPreviewText(content.MIME), Filename: copyPreviewText(content.Filename)}
		if content.Type == atom.Text {
			previewContent.Text = copyPreviewText(content.Text)
		} else {
			preview.Truncated = true
		}
		if len(content.Data) > 0 || content.URL != "" || content.AudioID != "" {
			preview.Truncated = true
		}
		preview.Content = append(preview.Content, previewContent)
	}
	return preview
}
