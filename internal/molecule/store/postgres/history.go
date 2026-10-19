package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

func (sessionStore *sessions) History(operationContext context.Context, sessionID atom.SessionID, query store.HistoryQuery) (atom.HistoryPage, error) {
	page := atom.HistoryPage{SessionID: sessionID, Messages: []atom.Message{}, Context: []atom.Message{}, Models: map[string]string{}, Sizes: map[string]int{}}
	query, operationError := query.Validate()
	if operationError != nil {
		return page, operationError
	}
	transaction, operationError := sessionStore.store.pool.BeginTx(operationContext, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if operationError != nil {
		return page, operationError
	}
	defer transaction.Rollback(context.WithoutCancel(operationContext))
	var parentID string
	operationError = transaction.QueryRow(operationContext, `SELECT history_revision,parent_id FROM sessions WHERE id=$1 AND NOT deleted`, string(sessionID)).Scan(&page.Revision, &parentID)
	if errors.Is(operationError, pgx.ErrNoRows) {
		return page, store.ErrSessionNotFound
	}
	if operationError != nil {
		return page, operationError
	}
	if query.Revision != nil && *query.Revision != page.Revision {
		return page, store.ErrHistoryChanged
	}
	if parentID != "" {
		var task string
		taskError := transaction.QueryRow(operationContext, `SELECT COALESCE(left(content->0->>'Text',4096),'') FROM messages WHERE session_id=$1 AND role='user' ORDER BY seq LIMIT 1`, string(sessionID)).Scan(&task)
		if taskError != nil && !errors.Is(taskError, pgx.ErrNoRows) {
			return page, taskError
		}
		page.Task = task
	}
	if operationError = transaction.QueryRow(operationContext, `SELECT COALESCE(MAX(seq),0) FROM messages WHERE session_id=$1`, string(sessionID)).Scan(&page.HeadSeq); operationError != nil {
		return page, operationError
	}
	latest := atom.ResponseUsageSnapshot{}
	var usage []byte
	operationError = transaction.QueryRow(operationContext, `SELECT message.id,message.seq,message.usage,COALESCE(usage.model_id,''),message.created_at FROM messages message LEFT JOIN usage_records usage ON usage.session_id=message.session_id AND usage.request_id=message.id WHERE message.session_id=$1 AND message.role='assistant' ORDER BY message.seq DESC LIMIT 1`, string(sessionID)).Scan(&latest.MessageID, &latest.Seq, &usage, &latest.Model, &latest.CreatedAt)
	if operationError == nil {
		if len(usage) > 0 {
			if operationError = json.Unmarshal(usage, &latest.Usage); operationError != nil {
				return page, operationError
			}
		}
		page.LatestResponse = &latest
	} else if !errors.Is(operationError, pgx.ErrNoRows) {
		return page, operationError
	}
	if query.MetadataOnly {
		return page, transaction.Commit(operationContext)
	}
	direction := "DESC"
	if query.After > 0 {
		direction = "ASC"
	}
	rows, operationError := transaction.Query(operationContext, `SELECT seq,256+COALESCE(octet_length(content::text),0)+COALESCE(octet_length(tool_calls::text),0)+COALESCE(octet_length(reasoning),0) FROM messages WHERE session_id=$1 AND ($2::bigint=0 OR seq<$2) AND ($3::bigint=0 OR seq>$3) ORDER BY seq `+direction+` LIMIT $4`, string(sessionID), query.Before, query.After, query.Limit)
	if operationError != nil {
		return page, operationError
	}
	var selected []int64
	bytes := 0
	for rows.Next() {
		var sequence int64
		var size int
		if operationError = rows.Scan(&sequence, &size); operationError != nil {
			rows.Close()
			return page, operationError
		}
		if len(selected) > 0 && bytes+size > store.HistoryPageBytes {
			break
		}
		selected = append(selected, sequence)
		bytes += size
	}
	operationError = rows.Err()
	rows.Close()
	if operationError != nil {
		return page, operationError
	}
	if len(selected) == 0 {
		return page, transaction.Commit(operationContext)
	}
	if query.After == 0 {
		slices.Reverse(selected)
	}
	page.FirstSeq = selected[0]
	page.LastSeq = selected[len(selected)-1]
	rows, operationError = transaction.Query(operationContext, `SELECT message.id,message.seq,message.role,message.content,message.tool_calls,message.tool_call_id,message.usage,message.created_at,message.reasoning,COALESCE(usage.model_id,'') FROM messages message LEFT JOIN usage_records usage ON usage.session_id=message.session_id AND usage.request_id=message.id WHERE message.session_id=$1 AND message.seq>=$2 AND message.seq<=$3 ORDER BY message.seq`, string(sessionID), page.FirstSeq, page.LastSeq)
	if operationError != nil {
		return page, operationError
	}
	for rows.Next() {
		message := atom.Message{SessionID: sessionID}
		var content, calls, report []byte
		var model string
		if operationError = rows.Scan(&message.ID, &message.Seq, &message.Role, &content, &calls, &message.ToolCallID, &report, &message.CreatedAt, &message.Reasoning, &model); operationError != nil {
			rows.Close()
			return page, operationError
		}
		if operationError = json.Unmarshal(content, &message.Content); operationError == nil {
			operationError = json.Unmarshal(calls, &message.ToolCalls)
		}
		if operationError == nil && len(report) > 0 {
			operationError = json.Unmarshal(report, &message.Usage)
		}
		if operationError != nil {
			rows.Close()
			return page, operationError
		}
		page.Messages = append(page.Messages, message)
		page.Sizes[message.ID] = store.PublicMessageSize(message)
		if model != "" {
			page.Models[message.ID] = model
		}
	}
	operationError = rows.Err()
	rows.Close()
	if operationError != nil {
		return page, operationError
	}
	if operationError = transaction.QueryRow(operationContext, `SELECT EXISTS(SELECT 1 FROM messages WHERE session_id=$1 AND seq<$2),EXISTS(SELECT 1 FROM messages WHERE session_id=$1 AND seq>$3)`, string(sessionID), page.FirstSeq, page.LastSeq).Scan(&page.HasOlder, &page.HasNewer); operationError != nil {
		return page, operationError
	}
	if len(page.Messages) > 0 && page.Messages[0].Role == atom.RoleTool {
		owner := atom.Message{SessionID: sessionID, Role: atom.RoleAssistant}
		var calls []byte
		operationError = transaction.QueryRow(operationContext, `SELECT id,seq,tool_calls,created_at FROM messages WHERE session_id=$1 AND seq<$2 AND role='assistant' ORDER BY seq DESC LIMIT 1`, string(sessionID), page.FirstSeq).Scan(&owner.ID, &owner.Seq, &calls, &owner.CreatedAt)
		if operationError == nil {
			if operationError = json.Unmarshal(calls, &owner.ToolCalls); operationError != nil {
				return page, operationError
			}
			owner = store.BoundaryToolOwner(owner, page.Messages)
			if len(owner.ToolCalls) > 0 {
				page.Context = append(page.Context, owner)
				page.Sizes[owner.ID] = store.PublicMessageSize(owner)
			}
		} else if !errors.Is(operationError, pgx.ErrNoRows) {
			return page, operationError
		}
	}
	return page, transaction.Commit(operationContext)
}
