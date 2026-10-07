package contextplugin

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

const searchTextBudget = 16 * 1024

type memorySearchInput struct {
	Query          string   `json:"query"`
	Categories     []string `json:"categories"`
	Compression    string   `json:"compression"`
	Source         string   `json:"source"`
	IncludeDeleted bool     `json:"include_deleted"`
	IncludeHistory bool     `json:"include_history"`
	Limit          int      `json:"limit"`
	Cursor         string   `json:"cursor,omitempty"`
}
type resultReference struct {
	Kind    string `json:"k"`
	ID      string `json:"i"`
	Version int    `json:"v"`
}
type memorySearchCursor struct {
	Workspace      string            `json:"w"`
	QueryHash      string            `json:"q"`
	Hits           []resultReference `json:"h"`
	Position       int               `json:"p"`
	SourcePosition int               `json:"s"`
	Offset         int               `json:"o"`
}
type memorySearchResult struct {
	Compression string `json:"compression"`
	Data        string `json:"data"`
	Historical  bool   `json:"historical,omitempty"`
	Deleted     bool   `json:"deleted,omitempty"`
}
type memorySearchReply struct {
	Matches    []memorySearchResult `json:"matches"`
	NextCursor string               `json:"next_cursor,omitempty"`
}

func (plugin *Plugin) searchMemory(operationContext context.Context, session atom.Session, input memorySearchInput) (memorySearchReply, error) {
	reply := memorySearchReply{Matches: []memorySearchResult{}}
	input.Query = strings.TrimSpace(input.Query)
	if len(input.Query) > 2048 {
		return reply, fmt.Errorf("query exceeds 2048 UTF-8 bytes")
	}
	var operationError error
	input.Categories, operationError = normalizeCategories(input.Categories)
	if operationError != nil {
		return reply, operationError
	}
	if input.Query == "" && len(input.Categories) == 0 {
		return reply, fmt.Errorf("memory search requires query text or categories")
	}
	if input.Compression == "" {
		input.Compression = "high"
	}
	if input.Source == "" {
		input.Source = "memories"
	}
	if input.Limit == 0 {
		input.Limit = 5
	}
	if input.Compression != "high" && input.Compression != "medium" && input.Compression != "low" && input.Compression != "raw" {
		return reply, fmt.Errorf("unsupported memory compression %q", input.Compression)
	}
	if input.Limit < 1 || input.Limit > 20 {
		return reply, fmt.Errorf("memory search limit is outside 1..20")
	}
	kinds := []string{"memory"}
	switch input.Source {
	case "memories":
	case "archive":
		kinds = []string{"source"}
	case "all":
		kinds = []string{"memory", "source"}
	default:
		return reply, fmt.Errorf("unsupported memory source %q", input.Source)
	}
	cursorText := input.Cursor
	input.Cursor = ""
	encoded, _ := json.Marshal(input)
	queryHash := sha256.Sum256(encoded)
	cursor := memorySearchCursor{Workspace: session.InstanceID, QueryHash: hex.EncodeToString(queryHash[:])}
	if cursorText != "" {
		if len(cursorText) > 4096 {
			return reply, fmt.Errorf("memory cursor exceeds 4096 bytes")
		}
		data, decodeError := base64.RawURLEncoding.DecodeString(cursorText)
		if decodeError != nil {
			return reply, fmt.Errorf("invalid memory cursor")
		}
		var previous memorySearchCursor
		if json.Unmarshal(data, &previous) != nil || previous.Workspace != cursor.Workspace || previous.QueryHash != cursor.QueryHash || len(previous.Hits) > 20 || previous.Position < 0 || previous.Position > len(previous.Hits) || previous.SourcePosition < 0 || previous.Offset < 0 {
			return reply, fmt.Errorf("memory cursor does not match this workspace and query")
		}
		cursor = previous
	} else {
		var vector []float64
		if input.Query != "" {
			chunks, embeddingError := plugin.embedQueryText(operationContext, session.InstanceID, "context.main_search", input.Query)
			if embeddingError != nil {
				return reply, embeddingError
			}
			vector, operationError = queryVector(chunks)
			if operationError != nil {
				return reply, operationError
			}
		}
		model := ""
		if len(vector) > 0 {
			model = plugin.embeddingModel
		}
		hits, searchError := plugin.database.Search(operationContext, session.InstanceID, searchQuery{Text: input.Query, Vector: vector, EmbeddingModel: model, Kinds: kinds, Categories: input.Categories, IncludeDeleted: input.IncludeDeleted, IncludeHistory: input.IncludeHistory, Limit: input.Limit})
		if searchError != nil {
			return reply, searchError
		}
		for _, hit := range hits {
			cursor.Hits = append(cursor.Hits, resultReference{hit.Kind, hit.ID, hit.Version})
		}
	}
	remaining := searchTextBudget
	for cursor.Position < len(cursor.Hits) && remaining > 0 {
		reference := cursor.Hits[cursor.Position]
		result, finished, readError := plugin.readSearchResult(operationContext, session, input, &cursor, remaining)
		if readError != nil {
			return reply, readError
		}
		if result.Data != "" {
			reply.Matches = append(reply.Matches, result)
			remaining -= len(result.Data)
		}
		if reference.Kind == "memory" && result.Data != "" && !result.Historical && !result.Deleted {
			if operationError := plugin.recordRetrieval(operationContext, session, reference.ID, input.Compression); operationError != nil {
				return reply, operationError
			}
		}
		if !finished {
			break
		}
		cursor.Position++
		cursor.SourcePosition = 0
		cursor.Offset = 0
	}
	if cursor.Position < len(cursor.Hits) {
		encoded, _ := json.Marshal(cursor)
		reply.NextCursor = base64.RawURLEncoding.EncodeToString(encoded)
		if len(reply.NextCursor) > 4096 {
			return memorySearchReply{}, fmt.Errorf("memory continuation exceeds 4096 bytes")
		}
	}
	if operationError := plugin.database.Metric(operationContext, session.InstanceID, "context.main_search", "queries", 1, 0); operationError != nil {
		return reply, operationError
	}
	return reply, nil
}

func (plugin *Plugin) readSearchResult(operationContext context.Context, session atom.Session, input memorySearchInput, cursor *memorySearchCursor, budget int) (memorySearchResult, bool, error) {
	reference := cursor.Hits[cursor.Position]
	result := memorySearchResult{Compression: input.Compression}
	var sourceIDs []int64
	if reference.Kind == "source" {
		identifier, operationError := strconv.ParseInt(reference.ID, 10, 64)
		if operationError != nil {
			return result, false, fmt.Errorf("invalid source cursor")
		}
		sourceIDs = []int64{identifier}
		result.Compression = "raw"
	} else if reference.Kind == "memory" {
		record, operationError := readValue[memoryRecord](operationContext, plugin.database, session.InstanceID, "memory_version", reference.ID+":"+strconv.Itoa(reference.Version))
		if operationError != nil {
			return result, false, operationError
		}
		if record.ID == "" {
			return result, true, nil
		}
		head, operationError := readValue[memoryRecord](operationContext, plugin.database, session.InstanceID, "memory", reference.ID)
		if operationError != nil {
			return result, false, operationError
		}
		result.Historical = head.Version != record.Version || head.Status == "superseded" || head.Status == "invalidated"
		result.Deleted = head.Status == "deleted"
		if result.Historical && !input.IncludeHistory || result.Deleted && !input.IncludeDeleted {
			return result, true, nil
		}
		if input.Compression == "raw" {
			sourceIDs = record.SourceIDs
		} else {
			text := record.Text.High
			if input.Compression == "medium" {
				text = record.Text.Medium
			}
			if input.Compression == "low" {
				text = record.Text.Low
			}
			if cursor.Offset > len(text) {
				return result, false, fmt.Errorf("memory cursor offset exceeds content")
			}
			part := boundedUTF8(text[cursor.Offset:], budget)
			result.Data = part
			cursor.Offset += len(part)
			return result, cursor.Offset == len(text), nil
		}
	} else {
		return result, false, fmt.Errorf("invalid memory cursor kind")
	}
	if cursor.SourcePosition > len(sourceIDs) {
		return result, false, fmt.Errorf("memory cursor source position exceeds content")
	}
	var text strings.Builder
	for cursor.SourcePosition < len(sourceIDs) {
		source, operationError := readValue[source](operationContext, plugin.database, session.InstanceID, "source", strconv.FormatInt(sourceIDs[cursor.SourcePosition], 10))
		if operationError != nil {
			return result, false, operationError
		}
		if source.ID == 0 || source.Deleted && !input.IncludeDeleted {
			cursor.SourcePosition++
			cursor.Offset = 0
			continue
		}
		result.Deleted = result.Deleted || source.Deleted
		content := "[" + string(source.Role) + "]\n" + source.Text + "\n"
		if cursor.Offset > len(content) {
			return result, false, fmt.Errorf("source cursor offset exceeds content")
		}
		part := boundedUTF8(content[cursor.Offset:], budget-text.Len())
		text.WriteString(part)
		cursor.Offset += len(part)
		if cursor.Offset < len(content) {
			result.Data = text.String()
			return result, false, nil
		}
		cursor.SourcePosition++
		cursor.Offset = 0
		if text.Len() >= budget {
			break
		}
	}
	result.Data = text.String()
	return result, cursor.SourcePosition == len(sourceIDs), nil
}

func boundedUTF8(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(text) <= limit {
		return text
	}
	for limit > 0 && !utf8.RuneStart(text[limit]) {
		limit--
	}
	return text[:limit]
}

func (plugin *Plugin) recordRetrieval(operationContext context.Context, session atom.Session, identifier, level string) error {
	return plugin.database.Transact(operationContext, session.InstanceID, func(database transaction) error {
		record, operationError := readTransactionValue[memoryRecord](database, "memory", identifier)
		if operationError != nil {
			return operationError
		}
		if record.ID == "" {
			return nil
		}
		record.LastRetrievedAt = time.Now().UTC()
		record.Retrievals++
		if operationError := database.Put("memory", identifier, record); operationError != nil {
			return operationError
		}
		if level != "low" && level != "raw" {
			return nil
		}
		view, operationError := loadView(database, session.ID)
		if operationError != nil {
			return operationError
		}
		if view.Deleted || view.Mutation != nil {
			return nil
		}
		view.Promote[identifier] = true
		return database.Put("view", string(session.ID), view)
	})
}

func (plugin *Plugin) listCategories(operationContext context.Context, workspaceID string) ([]string, error) {
	categories := map[string]bool{}
	after := ""
	for {
		documents, operationError := plugin.database.List(operationContext, workspaceID, "memory", after, 200)
		if operationError != nil {
			return nil, operationError
		}
		if len(documents) == 0 {
			break
		}
		for _, document := range documents {
			var record memoryRecord
			if operationError := json.Unmarshal(document, &record); operationError != nil {
				return nil, operationError
			}
			after = record.ID
			if record.Status != "active" && record.Status != "consolidated" {
				continue
			}
			for _, category := range record.Categories {
				categories[category] = true
			}
		}
		if len(documents) < 200 {
			break
		}
	}
	result := make([]string, 0, len(categories))
	for category := range categories {
		result = append(result, category)
	}
	sort.Strings(result)
	return result, nil
}
