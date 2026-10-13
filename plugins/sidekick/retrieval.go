package sidekick

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func (plugin *Plugin) retrieveSources(operationContext context.Context, session atom.Session, task atom.TaskState, user atom.Message, configuration configuration, state sessionState, runID string) ([]sourceData, map[string]int64, error) {
	query := task.Doing.Title + "\n" + task.Doing.Description
	visible := strings.Join(strings.Fields(plugin.currentModelView(session.ID, user.ID)), " ")
	seen := map[string]bool{}
	for _, event := range state.Events {
		for _, key := range event.Sources {
			seen[key] = true
		}
	}
	sources := []sourceData{}
	counts := map[string]int64{}
	if configuration.MemoryEnabled && plugin.services.Memory != nil {
		memories, operationError := plugin.services.Memory.SearchWorkspaceMemory(operationContext, harness.MemoryQuery{WorkspaceID: session.InstanceID, SourceSessionID: session.ID, Agent: agentName, RunID: runID, Query: boundedText(query, 2048), Compression: "medium", Limit: configuration.MemoryLimit, MaxBytes: configuration.MemoryBytes})
		counts["retrieval/memory_queries"]++
		if operationError != nil {
			if errors.Is(operationError, harness.ErrWorkspaceMemoryUnavailable) {
				counts["retrieval/memory_unavailable"]++
			} else {
				counts["retrieval/memory_errors"]++
				plugin.recordError(session.InstanceID, operationError)
			}
		} else {
			for _, memory := range memories {
				if text := strings.Join(strings.Fields(memory.Text), " "); text != "" && strings.Contains(visible, text) {
					counts["sources/already_present"]++
					continue
				}
				key := fmt.Sprintf("memory:%s:%d", memory.ID, memory.Version)
				if seen[key] {
					counts["sources/duplicates"]++
					continue
				}
				seen[key] = true
				copy := memory
				sources = append(sources, sourceData{Key: key, Origin: "workspace memory (medium compression)", Text: memory.Text, Memory: &copy})
				counts["sources/memories"]++
			}
		}
	}
	if configuration.FilesEnabled && plugin.services.Files != nil {
		files, operationError := plugin.services.Files.SearchWorkspaceFiles(operationContext, harness.WorkspaceFileQuery{WorkspaceID: session.InstanceID, SourceSessionID: session.ID, Query: query, Limit: configuration.FileLimit, MaxBytes: configuration.FileBytes})
		counts["retrieval/file_queries"]++
		counts["retrieval/files_examined"] += int64(files.FilesExamined)
		counts["retrieval/file_bytes"] += int64(files.BytesExamined)
		if files.Truncated {
			counts["retrieval/bounded_scans"]++
		}
		if operationError != nil {
			counts["retrieval/file_errors"]++
			plugin.recordError(session.InstanceID, operationError)
		} else {
			for _, file := range files.References {
				if text := strings.Join(strings.Fields(file.Text), " "); text != "" && strings.Contains(visible, text) {
					counts["sources/already_present"]++
					continue
				}
				key := fmt.Sprintf("file:%s:%d:%d:%s", file.Path, file.StartLine, file.EndLine, file.Version)
				if seen[key] {
					counts["sources/duplicates"]++
					continue
				}
				seen[key] = true
				copy := file
				sources = append(sources, sourceData{Key: key, Origin: fmt.Sprintf("%s:%d-%d", file.Path, file.StartLine, file.EndLine), Text: file.Text, File: &copy})
				counts["sources/files"]++
			}
		}
	}
	return sources, counts, operationContext.Err()
}

// The worker sees a bounded excerpt of the selected model view, including
// ephemeral memory that is deliberately absent from saved conversation history.
// Startup instructions retain their own trusted prompt boundary.
func (plugin *Plugin) captureModelView(input harness.ContextRequest, sourceUser string, maximum int) {
	var parts []string
	remaining := maximum
	for index, message := range input.Request.Messages {
		if index == 0 || !message.Ephemeral {
			continue
		}
		text := boundedText(messageText(message), remaining)
		if text != "" {
			parts = append(parts, text)
			remaining -= len(text)
		}
		if remaining == 0 {
			break
		}
	}
	for index := len(input.Request.Messages) - 1; index >= 0 && remaining > 0; index-- {
		message := input.Request.Messages[index]
		if message.Ephemeral {
			continue
		}
		text := boundedText(messageText(message), remaining)
		if text != "" {
			parts = append(parts, text)
			remaining -= len(text)
		}
	}
	plugin.mutex.Lock()
	plugin.views[input.Session.ID] = modelView{sourceUser: sourceUser, text: strings.Join(parts, "\n")}
	plugin.mutex.Unlock()
}

func (plugin *Plugin) currentModelView(sessionID atom.SessionID, sourceUser string) string {
	plugin.mutex.Lock()
	defer plugin.mutex.Unlock()
	view := plugin.views[sessionID]
	if view.sourceUser != sourceUser {
		return ""
	}
	return view.text
}

func messageText(message atom.Message) string {
	var result strings.Builder
	for _, content := range message.Content {
		if content.Type == atom.Text {
			if result.Len() > 0 {
				result.WriteByte('\n')
			}
			result.WriteString(content.Text)
		}
	}
	return result.String()
}
func boundedText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	return value[:limit]
}

func (plugin *Plugin) verifyHintSources(operationContext context.Context, session atom.Session, hint preparedHint) (bool, error) {
	if len(hint.Files) > 0 {
		if plugin.services.Files == nil {
			return false, nil
		}
		valid, operationError := plugin.services.Files.VerifyWorkspaceFiles(operationContext, session.InstanceID, session.ID, hint.Files)
		if operationError != nil || !valid {
			return valid, operationError
		}
	}
	if len(hint.Memories) > 0 {
		if verifier, supported := plugin.services.Memory.(harness.WorkspaceMemoryVerifier); supported {
			return verifier.VerifyWorkspaceMemory(operationContext, session.InstanceID, session.ID, hint.Memories)
		}
	}
	return true, nil
}
