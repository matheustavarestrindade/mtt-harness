package contextplugin

import "github.com/matheustavarestrindade/mtt-harness/atom"

// contextGroups preserves assistant tool plans with every matching result. The
// latest input and the two newest groups are protected even within a long turn.
func contextGroups(messages []atom.Message, view sessionView) ([][]int64, map[int64]bool) {
	var groups [][]int64
	protected := map[int64]bool{}
	lastUser, lastInput := int64(0), int64(0)
	for position := 0; position < len(messages); position++ {
		message := messages[position]
		identifier := view.Sources[message.ID]
		if identifier == 0 || message.Ephemeral || view.Removed[message.ID] {
			continue
		}
		group := []int64{identifier}
		if message.Role == atom.RoleSystem {
			protected[identifier] = true
		}
		if message.Role == atom.RoleUser {
			lastUser = identifier
			lastInput = identifier
		}
		if message.Role == atom.RoleRuntime {
			lastInput = identifier
		}
		if len(message.ToolCalls) > 0 {
			pending := map[string]bool{}
			for _, call := range message.ToolCalls {
				pending[call.ID] = true
			}
			for next := position + 1; next < len(messages) && messages[next].Role == atom.RoleTool; next++ {
				result := messages[next]
				if _, found := pending[result.ToolCallID]; !found {
					break
				}
				delete(pending, result.ToolCallID)
				if resultID := view.Sources[result.ID]; resultID != 0 && !view.Removed[result.ID] {
					group = append(group, resultID)
				}
				position = next
			}
			if len(pending) > 0 {
				for _, member := range group {
					protected[member] = true
				}
			}
		} else if message.Role == atom.RoleTool {
			protected[identifier] = true
		}
		groups = append(groups, group)
	}
	if lastUser != 0 {
		protected[lastUser] = true
	}
	if lastInput != 0 {
		protected[lastInput] = true
	}
	for index := max(0, len(groups)-2); index < len(groups); index++ {
		for _, identifier := range groups[index] {
			protected[identifier] = true
		}
	}
	return groups, protected
}

func groupContainsProtected(group []int64, protected map[int64]bool) bool {
	for _, identifier := range group {
		if protected[identifier] {
			return true
		}
	}
	return false
}
