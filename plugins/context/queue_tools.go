package contextplugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type dropInput struct {
	MessageIDs []int64  `json:"message_ids"`
	Remember   *bool    `json:"remember"`
	Categories []string `json:"categories"`
}
type rememberInput struct {
	Text       string   `json:"text"`
	Categories []string `json:"categories"`
}

func normalizeCategories(categories []string) ([]string, error) {
	if len(categories) > 16 {
		return nil, fmt.Errorf("categories exceeds 16 values")
	}
	seen := map[string]bool{}
	result := []string{}
	for _, category := range categories {
		category = strings.ToLower(strings.TrimSpace(category))
		if len(category) < 1 || len(category) > 48 {
			return nil, fmt.Errorf("category length is outside 1..48 bytes")
		}
		for _, character := range category {
			if !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-' || character == '_') {
				return nil, fmt.Errorf("category %q contains an unsupported character", category)
			}
		}
		if !seen[category] {
			seen[category] = true
			result = append(result, category)
		}
	}
	sort.Strings(result)
	return result, nil
}

func (plugin *Plugin) queueDrop(operationContext context.Context, session atom.Session, input dropInput) (any, error) {
	if len(input.MessageIDs) == 0 || len(input.MessageIDs) > 128 || input.Remember == nil {
		return nil, fmt.Errorf("ctx_drop requires 1..128 message IDs and remember")
	}
	categories, operationError := normalizeCategories(input.Categories)
	if operationError != nil {
		return nil, operationError
	}
	var result memoryJob
	operationError = plugin.database.Transact(operationContext, session.InstanceID, func(database transaction) error {
		view, operationError := loadView(database, session.ID)
		if operationError != nil {
			return operationError
		}
		if view.Deleted || view.Mutation != nil {
			return fmt.Errorf("session context is fenced")
		}
		selected := map[int64]bool{}
		available := map[int64]bool{}
		for _, identifier := range view.Available {
			available[identifier] = true
		}
		for _, identifier := range input.MessageIDs {
			if identifier <= 0 || selected[identifier] {
				return fmt.Errorf("invalid or duplicate message ID %d", identifier)
			}
			if !available[identifier] {
				return fmt.Errorf("message %d is not in the current context view", identifier)
			}
			if view.Protected[identifier] {
				return fmt.Errorf("message %d belongs to protected context", identifier)
			}
			selected[identifier] = true
		}
		for _, group := range view.Groups {
			touched := false
			for _, identifier := range group {
				touched = touched || selected[identifier]
			}
			if touched {
				for _, identifier := range group {
					if !selected[identifier] {
						return fmt.Errorf("selection contains part of a tool group; group message IDs: %v", group)
					}
				}
			}
		}
		result, operationError = enqueueReduction(database, view, input.MessageIDs, *input.Remember, categories, false)
		return operationError
	})
	if operationError != nil {
		return nil, operationError
	}
	plugin.signalWork()
	return map[string]any{"state": result.Status, "queued_messages": len(result.SourceIDs), "remember": result.Remember}, nil
}

func enqueueReduction(database transaction, view sessionView, identifiers []int64, remember bool, categories []string, urgent bool) (memoryJob, error) {
	selected := append([]int64(nil), identifiers...)
	sort.Slice(selected, func(first, second int) bool { return selected[first] < selected[second] })
	identity, _ := json.Marshal([]any{view.SessionID, view.Epoch, selected, remember, categories})
	digest := sha256.Sum256(identity)
	identifier := hex.EncodeToString(digest[:])
	previous, operationError := readTransactionValue[memoryJob](database, "job", identifier)
	if operationError != nil {
		return memoryJob{}, operationError
	}
	if previous.ID != "" {
		if urgent && previous.Priority > 0 {
			previous.Priority = 0
			operationError = database.Put("job", identifier, previous)
		}
		return previous, operationError
	}
	priority := 20
	if urgent {
		priority = 0
	}
	job := memoryJob{ID: identifier, SessionID: view.SessionID, Epoch: view.Epoch, Kind: "reduce", Agent: "context.compactor", Status: "pending", Priority: priority, SourceIDs: selected, Remember: remember, Categories: categories, CreatedAt: time.Now().UTC()}
	// Nearby public sources supply approval context; only SourceIDs are selected
	// for removal. Including neighbors never changes the reduction selection.
	selectedSet := map[int64]bool{}
	for _, identifier := range selected {
		selectedSet[identifier] = true
	}
	for position, identifier := range view.Available {
		if !selectedSet[identifier] {
			continue
		}
		for neighbor := max(0, position-2); neighbor < min(len(view.Available), position+3); neighbor++ {
			job.ContextIDs = append(job.ContextIDs, view.Available[neighbor])
		}
	}
	job.ContextIDs = uniqueSourceIDs(job.ContextIDs)
	return job, database.Put("job", identifier, job)
}

func (plugin *Plugin) queueRemember(operationContext context.Context, session atom.Session, callID string, input rememberInput) (any, error) {
	if strings.TrimSpace(input.Text) == "" || len(input.Text) > 8192 {
		return nil, fmt.Errorf("remember text must contain 1..8192 UTF-8 bytes")
	}
	categories, operationError := normalizeCategories(input.Categories)
	if operationError != nil {
		return nil, operationError
	}
	operationError = plugin.database.Transact(operationContext, session.InstanceID, func(database transaction) error {
		view, operationError := loadView(database, session.ID)
		if operationError != nil {
			return operationError
		}
		if view.Deleted || view.Mutation != nil {
			return fmt.Errorf("session context is fenced")
		}
		identity := sha256.Sum256([]byte(string(session.ID) + "\x00remember\x00" + callID))
		identifier := hex.EncodeToString(identity[:])
		previous, operationError := readTransactionValue[memoryJob](database, "job", identifier)
		if operationError != nil {
			return operationError
		}
		if previous.ID != "" {
			return nil
		}
		sourceID, operationError := database.NextSourceID()
		if operationError != nil {
			return operationError
		}
		entry := source{ID: sourceID, SessionID: session.ID, MessageID: "remember:" + callID, Role: atom.RoleAssistant, Text: input.Text, CreatedAt: time.Now().UTC()}
		if operationError := database.Put("source", strconv.FormatInt(sourceID, 10), entry); operationError != nil {
			return operationError
		}
		job := memoryJob{ID: identifier, SessionID: session.ID, Epoch: view.Epoch, Kind: "remember", Agent: "context.memory_writer", Status: "pending", Priority: 10, SourceIDs: []int64{sourceID}, ContextIDs: append([]int64(nil), view.Available[max(0, len(view.Available)-12):]...), Remember: true, Text: input.Text, Categories: categories, CreatedAt: time.Now().UTC()}
		return database.Put("job", identifier, job)
	})
	if operationError != nil {
		return nil, operationError
	}
	plugin.signalWork()
	return map[string]any{"state": "queued"}, nil
}

func (plugin *Plugin) requestWrapup(operationContext context.Context, session atom.Session) (any, error) {
	ready, pending := 0, 0
	operationError := plugin.database.Transact(operationContext, session.InstanceID, func(database transaction) error {
		view, operationError := loadView(database, session.ID)
		if operationError != nil {
			return operationError
		}
		if view.Deleted || view.Mutation != nil {
			return fmt.Errorf("session context is fenced")
		}
		jobs, operationError := readTransactionValues[memoryJob](database, "job")
		if operationError != nil {
			return operationError
		}
		for _, job := range jobs {
			if job.SessionID != session.ID || job.Epoch != view.Epoch || job.Kind != "reduce" {
				continue
			}
			if job.Status == "ready" {
				ready++
			}
			if job.Status == "pending" || job.Status == "running" {
				pending++
				if job.Priority > 0 {
					job.Priority = 0
					if operationError := database.Put("job", job.ID, job); operationError != nil {
						return operationError
					}
				}
			}
		}
		view.WrapRequested = true
		return database.Put("view", string(session.ID), view)
	})
	if operationError != nil {
		return nil, operationError
	}
	plugin.signalWork()
	return map[string]any{"state": "requested", "ready_jobs": ready, "pending_jobs": pending}, nil
}

func uniqueSourceIDs(identifiers []int64) []int64 {
	seen := map[int64]bool{}
	result := []int64{}
	for _, identifier := range identifiers {
		if !seen[identifier] {
			seen[identifier] = true
			result = append(result, identifier)
		}
	}
	return result
}
