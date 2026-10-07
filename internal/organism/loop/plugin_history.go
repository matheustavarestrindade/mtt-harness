package loop

import (
	"context"
	"sort"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

// beginPluginHistoryChange runs in a lifecycle worker after admission is fenced
// and previous writers are joined. Owner goroutines never invoke these hooks.
func (agentLoop *Loop) beginPluginHistoryChange(operationContext context.Context, workspaceID string, kind harness.HistoryChangeKind, identifiers []atom.SessionID, keepMessageID string) (func(context.Context, bool) error, error) {
	if agentLoop.configuration.Plugins == nil {
		return func(context.Context, bool) error { return nil }, nil
	}
	change := harness.HistoryChange{Kind: kind, WorkspaceID: workspaceID, KeepMessageID: keepMessageID}
	identifiers = append([]atom.SessionID(nil), identifiers...)
	sort.Slice(identifiers, func(first, second int) bool { return identifiers[first] < identifiers[second] })
	for _, identifier := range identifiers {
		session, operationError := agentLoop.configuration.Store.Sessions().Get(operationContext, identifier)
		if operationError != nil {
			return nil, operationError
		}
		messages, operationError := agentLoop.configuration.Store.Sessions().Messages(operationContext, identifier)
		if operationError != nil {
			return nil, operationError
		}
		publicMessages := append([]atom.Message(nil), messages...)
		for position := range publicMessages {
			publicMessages[position].ProviderState = nil
		}
		change.Snapshots = append(change.Snapshots, harness.HistorySnapshot{Session: session, Messages: publicMessages})
	}
	return agentLoop.configuration.Plugins.BeginHistoryChange(operationContext, change)
}
