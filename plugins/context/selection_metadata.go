package contextplugin

import (
	"encoding/json"
	"regexp"
	"sort"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

var echoedContextLabel = regexp.MustCompile(`^(?:\s*\[context_message id=[0-9]+ role=assistant\]\s*)+`)

// Old model replies can contain copied harness labels. Keep those artifacts out
// of subsequent model requests without rewriting the stored conversation.
func withoutEchoedContextLabel(message atom.Message) atom.Message {
	if message.Role != atom.RoleAssistant {
		return message
	}
	for position, content := range message.Content {
		if content.Type != atom.Text {
			continue
		}
		cleaned := echoedContextLabel.ReplaceAllString(content.Text, "")
		if cleaned != content.Text {
			message.Content = append([]atom.Content(nil), message.Content...)
			message.Content[position].Text = cleaned
		}
		break
	}
	return message
}

// References are lower-priority tail data, never examples of assistant prose.
// No reference, role header, or mutable protection flag enters message content.
func appendSelectionMetadata(messages []atom.Message, view sessionView) []atom.Message {
	var references [][2]any
	for _, message := range messages {
		identifier := view.Sources[message.ID]
		if identifier == 0 || message.Ephemeral || message.Role == atom.RoleSystem {
			continue
		}
		references = append(references, [2]any{identifier, message.Role})
	}
	if len(references) == 0 {
		return messages
	}
	groups, protected := contextGroups(messages, view)
	protectedIDs := make([]int64, 0, len(protected))
	for identifier, value := range protected {
		if value {
			protectedIDs = append(protectedIDs, identifier)
		}
	}
	sort.Slice(protectedIDs, func(first, second int) bool { return protectedIDs[first] < protectedIDs[second] })
	encoded, _ := json.Marshal(struct {
		Messages  [][2]any  `json:"messages"`
		Groups    [][]int64 `json:"groups"`
		Protected []int64   `json:"protected"`
	}{references, groups, protectedIDs})
	return append(messages, atom.Message{SessionID: view.SessionID, Role: atom.RoleRuntime, Ephemeral: true, Content: []atom.Content{{Type: atom.Text, Text: "Context selection data: " + string(encoded)}}})
}
