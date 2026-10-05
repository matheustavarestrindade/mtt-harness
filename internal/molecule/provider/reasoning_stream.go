package provider

import (
	"fmt"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

// Reasoning deltas, item.done, and response.completed can all contain the same
// public text. Keys identify one summary/content part so it is emitted once.
type reasoningStream struct {
	parts map[string]*strings.Builder
	send  func(atom.ResponsePart) bool
}

func reasoningPartKey(outputIndex int, kind string, partIndex int) string {
	return fmt.Sprintf("%d/%s/%d", outputIndex, kind, partIndex)
}

func (reasoning *reasoningStream) appendDelta(key, text string) bool {
	if text == "" {
		return true
	}
	if reasoning.parts == nil {
		reasoning.parts = map[string]*strings.Builder{}
	}
	part, present := reasoning.parts[key]
	if !present {
		part = &strings.Builder{}
		reasoning.parts[key] = part
	}
	part.WriteString(text)
	if !present && len(reasoning.parts) > 1 {
		text = "\n\n" + text
	}
	return reasoning.send(atom.ResponsePart{Reasoning: text})
}

func (reasoning *reasoningStream) completePart(key, text string) bool {
	previous := ""
	if part := reasoning.parts[key]; part != nil {
		previous = part.String()
	}
	if !strings.HasPrefix(text, previous) {
		// Keep already-delivered text if an upstream final summary was rewritten.
		// Appending that whole summary would duplicate the reasoning in the UI.
		return true
	}
	return reasoning.appendDelta(key, strings.TrimPrefix(text, previous))
}
