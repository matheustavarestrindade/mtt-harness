package contextplugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type rememberReceipt struct {
	MemoryID string `json:"memory_id"`
}

// saveMemory commits the caller's text and its search index together. Model
// inference is reserved for context extraction and historian work, not saves.
func (plugin *Plugin) saveMemory(operationContext context.Context, session atom.Session, callID string, input rememberInput) (any, error) {
	if strings.TrimSpace(input.Text) == "" || len(input.Text) > 8192 || !utf8.ValidString(input.Text) {
		return nil, fmt.Errorf("remember text must contain 1..8192 UTF-8 bytes")
	}
	if input.OldText != nil && (strings.TrimSpace(*input.OldText) == "" || len(*input.OldText) > 8192 || !utf8.ValidString(*input.OldText)) {
		return nil, fmt.Errorf("old_text must contain 1..8192 UTF-8 bytes")
	}
	categories, operationError := normalizeCategories(input.Categories)
	if operationError != nil {
		return nil, operationError
	}
	identity := sha256.Sum256([]byte(string(session.ID) + "\x00remember\x00" + callID))
	identifier := hex.EncodeToString(identity[:])
	view, operationError := readValue[sessionView](operationContext, plugin.database, session.InstanceID, "view", string(session.ID))
	if operationError != nil {
		return nil, operationError
	}
	if view.Deleted || view.Mutation != nil {
		return nil, fmt.Errorf("session context is fenced")
	}
	receipt, operationError := readValue[rememberReceipt](operationContext, plugin.database, session.InstanceID, "remember", identifier)
	if operationError != nil {
		return nil, operationError
	}
	if receipt.MemoryID != "" {
		return map[string]string{"state": "saved"}, nil
	}
	vectors, operationError := plugin.embedText(operationContext, session.InstanceID, "context.remember", input.Text)
	if operationError != nil {
		return nil, operationError
	}
	operationError = plugin.database.Transact(operationContext, session.InstanceID, func(database transaction) error {
		current, operationError := loadView(database, session.ID)
		if operationError != nil {
			return operationError
		}
		if current.Deleted || current.Mutation != nil || current.Epoch != view.Epoch {
			return fmt.Errorf("session context changed before memory save")
		}
		receipt, operationError := readTransactionValue[rememberReceipt](database, "remember", identifier)
		if operationError != nil || receipt.MemoryID != "" {
			return operationError
		}
		now := time.Now().UTC()
		record := memoryRecord{ID: identifier, Version: 1, Title: boundedUTF8(input.Text, 160), Categories: categories, Text: summaries{Low: input.Text, Medium: input.Text, High: input.Text}, Kind: "note", Status: "active", CreatedAt: now, UpdatedAt: now}
		if input.OldText != nil {
			memories, operationError := readTransactionValues[memoryRecord](database, "memory")
			if operationError != nil {
				return operationError
			}
			var previous memoryRecord
			for _, candidate := range memories {
				if (candidate.Status != "active" && candidate.Status != "consolidated") || candidate.Text.Low != *input.OldText {
					continue
				}
				if previous.ID != "" {
					return fmt.Errorf("old_text matches more than one current memory")
				}
				previous = candidate
			}
			if previous.ID == "" {
				return fmt.Errorf("old_text does not match a current memory")
			}
			record.ID, record.Version, record.CreatedAt = previous.ID, previous.Version+1, previous.CreatedAt
			record.Important = previous.Important
			if input.Categories == nil {
				record.Categories = previous.Categories
			}
			if previous.Kind == "idea" {
				if operationError := activateSupportedChildren(database, previous.Children); operationError != nil {
					return operationError
				}
			}
			if operationError := database.MarkVectors("memory", previous.ID, false, true); operationError != nil {
				return operationError
			}
			if operationError := invalidateIdeas(database, memories, previous.ID, record.ID); operationError != nil {
				return operationError
			}
		}
		sourceID, operationError := database.NextSourceID()
		if operationError != nil {
			return operationError
		}
		entry := source{ID: sourceID, SessionID: session.ID, MessageID: "remember:" + callID, Role: atom.RoleAssistant, Text: input.Text, CreatedAt: now, Indexed: true, IndexedChunks: len(vectors), IndexModel: plugin.embeddingModel}
		record.SourceIDs = []int64{sourceID}
		if operationError := database.Put("source", strconv.FormatInt(sourceID, 10), entry); operationError != nil {
			return operationError
		}
		if operationError := database.Put("memory", record.ID, record); operationError != nil {
			return operationError
		}
		if operationError := database.Put("memory_version", record.ID+":"+strconv.Itoa(record.Version), record); operationError != nil {
			return operationError
		}
		if operationError := database.ReplaceVectors("memory", record.ID, record.Version, plugin.embeddingModel, record.Categories, vectors, false, false); operationError != nil {
			return operationError
		}
		if operationError := database.ReplaceVectors("source", strconv.FormatInt(sourceID, 10), 1, plugin.embeddingModel, record.Categories, vectors, false, false); operationError != nil {
			return operationError
		}
		if operationError := database.AddMetric("context.remember", "saved_memories", 1, 0); operationError != nil {
			return operationError
		}
		return database.Put("remember", identifier, rememberReceipt{MemoryID: record.ID})
	})
	if operationError != nil {
		return nil, operationError
	}
	return map[string]string{"state": "saved"}, nil
}
