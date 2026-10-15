package sidekick

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

const activityTimeFormat = "2006-01-02T15:04:05.000000000Z"

func (database *postgresRepository) ReadActivity(operationContext context.Context, workspaceID, sessionID string, cursor activityCursor, limit int) (activitySnapshot, error) {
	transaction, operationError := database.pool.BeginTx(operationContext, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if operationError != nil {
		return activitySnapshot{}, operationError
	}
	defer transaction.Rollback(context.WithoutCancel(operationContext))
	var data []byte
	operationError = transaction.QueryRow(operationContext, `SELECT data FROM sidekick.sessions WHERE workspace_id=$1 AND session_id=$2`, workspaceID, sessionID).Scan(&data)
	if errors.Is(operationError, pgx.ErrNoRows) {
		return activitySnapshot{}, nil
	}
	if operationError != nil {
		return activitySnapshot{}, operationError
	}
	var state sessionState
	if operationError = json.Unmarshal(data, &state); operationError != nil {
		return activitySnapshot{}, operationError
	}
	result := activitySnapshot{Revision: state.Revision, Epoch: state.ActivityEpoch}
	if state.Deleted {
		return result, nil
	}
	if state.Fenced || (cursor.ID != "" && cursor.Epoch != state.ActivityEpoch) {
		return result, harness.ErrPluginActivityChanged
	}
	query := `SELECT data->'hint' FROM sidekick.runs WHERE workspace_id=$1 AND session_id=$2 AND data ? 'hint'`
	arguments := []any{workspaceID, sessionID}
	if cursor.ID != "" {
		query += ` AND (data->>'hint_at',id)<($3::text,$4::text)`
		arguments = append(arguments, cursor.CreatedAt.UTC().Format(activityTimeFormat), cursor.ID)
	}
	if cursor.ID != "" {
		query += ` ORDER BY data->>'hint_at' DESC,id DESC LIMIT $5`
	} else {
		query += ` ORDER BY data->>'hint_at' DESC,id DESC LIMIT $3`
	}
	arguments = append(arguments, limit)
	rows, operationError := transaction.Query(operationContext, query, arguments...)
	if operationError != nil {
		return result, operationError
	}
	var delivered []hintEvent
	for rows.Next() {
		if operationError = rows.Scan(&data); operationError != nil {
			rows.Close()
			return result, operationError
		}
		var event hintEvent
		if operationError = json.Unmarshal(data, &event); operationError != nil {
			rows.Close()
			return result, operationError
		}
		delivered = append(delivered, event)
	}
	operationError = rows.Err()
	rows.Close()
	if operationError != nil {
		return result, operationError
	}
	// Older versions retained only active insertions. They remain readable when
	// their run metadata has no hint; never synthesize lost or undelivered text.
	result.Events = mergeActivityEvents(delivered, state.Events, cursor, limit)
	return result, transaction.Commit(operationContext)
}

func mergeActivityEvents(delivered, legacy []hintEvent, cursor activityCursor, limit int) []hintEvent {
	known := map[string]bool{}
	result := make([]hintEvent, 0, len(delivered)+len(legacy))
	for _, events := range [][]hintEvent{delivered, legacy} {
		for _, event := range events {
			if known[event.ID] || event.ID == "" {
				continue
			}
			if cursor.ID != "" && (event.CreatedAt.After(cursor.CreatedAt) || (event.CreatedAt.Equal(cursor.CreatedAt) && event.ID >= cursor.ID)) {
				continue
			}
			known[event.ID] = true
			result = append(result, event)
		}
	}
	slices.SortFunc(result, func(first, second hintEvent) int {
		if order := second.CreatedAt.Compare(first.CreatedAt); order != 0 {
			return order
		}
		if first.ID < second.ID {
			return 1
		}
		if first.ID > second.ID {
			return -1
		}
		return 0
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}
