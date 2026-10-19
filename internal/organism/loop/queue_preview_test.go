package loop

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func TestQueuedPreviewsKeepRoleAndLiteralText(test *testing.T) {
	message := atom.Message{ID: "queued", SessionID: "session", Role: atom.RoleUser, CreatedAt: time.Now(), Content: []atom.Content{{Type: atom.Text, Text: "Please keep this request 🌍\nsecond line"}}}
	preview := previewQueuedMessage(message)
	if preview.Truncated || preview.Role != message.Role || preview.ID != message.ID || preview.SessionID != message.SessionID || preview.Content[0].Text != message.Content[0].Text {
		test.Fatalf("preview changed pending input: %+v", preview)
	}
	preview.Content[0].Text = "changed preview"
	if message.Content[0].Text == preview.Content[0].Text {
		test.Fatal("preview aliases pending content")
	}
}

func TestQueuedPreviewsBoundUTF8AndOmitMediaBytes(test *testing.T) {
	message := atom.Message{ID: "large", SessionID: "session", Role: atom.RoleRuntime, Content: []atom.Content{{Type: atom.Text, Text: strings.Repeat("🌍", 100000)}, {Type: atom.Image, Data: make([]byte, 1<<20), URL: "data:image/png;base64," + strings.Repeat("A", 1<<20), MIME: "image/png", Filename: "image.png"}}}
	preview := previewQueuedMessage(message)
	if !preview.Truncated || preview.Role != atom.RoleRuntime || !utf8.ValidString(preview.Content[0].Text) || len(preview.Content[0].Text) > queuedPreviewBytes {
		test.Fatal("preview lost role or exceeded the text budget")
	}
	media := previewQueuedMessage(atom.Message{Content: message.Content[1:]})
	if !media.Truncated || media.Content[0].Filename != "image.png" || media.Content[0].MIME != "image/png" || len(media.Content[0].Data) != 0 || media.Content[0].URL != "" {
		test.Fatal("media preview included data or lost its descriptor")
	}
	parts := make([]atom.Content, 10000)
	for index := range parts {
		parts[index] = atom.Content{Type: atom.Text, Text: "x"}
	}
	bounded := previewQueuedMessage(atom.Message{Content: parts})
	if !bounded.Truncated || len(bounded.Content) > queuedPreviewParts {
		test.Fatal("preview did not bound the content-part count")
	}
}
