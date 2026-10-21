package sidekick

import (
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

// Language cues come from human input, not generated task names, previous hints,
// thinking, or retrieved material. Keep the reference small and newest first.
func buildUserLanguageReference(user atom.Message, messages []atom.Message) []string {
	const maximumReferences = 3
	const referenceBytes = 512
	result := make([]string, 0, maximumReferences)
	if user.Role != atom.RoleUser || user.Ephemeral {
		return result
	}
	if text := strings.TrimSpace(messageText(user)); text != "" {
		result = append(result, boundedText(text, referenceBytes))
	}
	anchor := len(messages)
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].ID == user.ID {
			anchor = index
			break
		}
	}
	for index := anchor - 1; index >= 0 && len(result) < maximumReferences; index-- {
		message := messages[index]
		if message.ID == user.ID || message.Role != atom.RoleUser || message.Ephemeral {
			continue
		}
		if text := strings.TrimSpace(messageText(message)); text != "" {
			result = append(result, boundedText(text, referenceBytes))
		}
	}
	return result
}
