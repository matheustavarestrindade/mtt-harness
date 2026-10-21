package watcher

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

var variableName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
var contextVariable = regexp.MustCompile(`^last_([0-9]+)(k?)_context$`)

func renderVariables(template string, resolve func(string) (string, error)) (string, error) {
	var output strings.Builder
	for index := 0; index < len(template); {
		if strings.HasPrefix(template[index:], "{{") {
			output.WriteByte('{')
			index += 2
			continue
		}
		if strings.HasPrefix(template[index:], "}}") {
			output.WriteByte('}')
			index += 2
			continue
		}
		if template[index] == '{' {
			end := strings.IndexByte(template[index+1:], '}')
			if end >= 0 {
				name := template[index+1 : index+1+end]
				if variableName.MatchString(name) {
					value, operationError := resolve(name)
					if operationError != nil {
						return "", operationError
					}
					output.WriteString(value)
					index += end + 2
					continue
				}
			}
		}
		output.WriteByte(template[index])
		index++
	}
	return output.String(), nil
}
func contextTokens(name string, defaultTokens int) (int, bool, error) {
	if name == "last_context" {
		return defaultTokens, true, nil
	}
	match := contextVariable.FindStringSubmatch(name)
	if match == nil {
		return 0, false, nil
	}
	count, operationError := strconv.Atoi(match[1])
	if operationError != nil {
		return 0, true, fmt.Errorf("invalid context variable %q", name)
	}
	if match[2] == "k" {
		if count > 1024 {
			return 0, true, fmt.Errorf("context variable exceeds 1048576 tokens")
		}
		count *= 1024
	}
	if count < 1 || count > 1048576 {
		return 0, true, fmt.Errorf("context variable must select 1-1048576 tokens")
	}
	return count, true, nil
}
func validateTemplate(template string) error {
	_, operationError := renderVariables(template, func(name string) (string, error) {
		if _, matched, operationError := contextTokens(name, 32768); matched {
			return "", operationError
		}
		switch name {
		case "todo", "doing", "event", "workspace", "permission_request", "tool_call":
			return "", nil
		}
		return "", fmt.Errorf("unknown watcher variable %q", name)
	})
	return operationError
}
func templateMaximumTokens(value definition) int {
	maximum := value.ContextTokens
	_, _ = renderVariables(value.Prompt, func(name string) (string, error) {
		if tokens, found, operationError := contextTokens(name, value.ContextTokens); found && operationError == nil {
			maximum = max(maximum, tokens)
		}
		return "", nil
	})
	return maximum
}

// selectTranscript measures candidate text through the configured provider's
// counter/estimator. Complete message rows win; a too-large final row becomes an
// explicitly marked UTF-8 excerpt rather than silently violating the token limit.
func selectTranscript(rows []string, limit int, measure func(string) (int, error)) (string, bool, error) {
	if len(rows) == 0 {
		return "", false, nil
	}
	first, last := 0, len(rows)
	best := ""
	omitted := false
	for first <= last {
		middle := (first + last) / 2
		text := strings.Join(rows[middle:], "\n\n")
		if middle > 0 && middle < len(rows) {
			text = "Context excerpt: earlier messages omitted.\n\n" + text
		}
		tokens, operationError := measure(text)
		if operationError != nil {
			return "", false, operationError
		}
		if tokens <= limit {
			best = text
			omitted = middle > 0
			last = middle - 1
		} else {
			first = middle + 1
		}
	}
	if best != "" {
		return best, omitted, nil
	}
	text := rows[len(rows)-1]
	header, body, hasHeader := strings.Cut(text, "\n")
	if hasHeader {
		text = body
		header += "\n"
	} else {
		header = ""
	}
	lower, upper := 0, len(text)
	for lower <= upper {
		length := (lower + upper) / 2
		start := len(text) - length
		for start < len(text) && !utf8.RuneStart(text[start]) {
			start++
		}
		excerpt := header + "[Earlier content omitted; message excerpt]\n" + text[start:]
		tokens, operationError := measure(excerpt)
		if operationError != nil {
			return "", false, operationError
		}
		if tokens <= limit {
			best = excerpt
			lower = length + 1
		} else {
			upper = length - 1
		}
	}
	return best, true, nil
}

func (plugin *Plugin) renderInput(operationContext context.Context, value definition, source harness.ReviewSource, event observation) (string, harness.RequestBudget, bool, error) {
	phase := map[string]string{"context_growth": "active_request", "permission_request": "pending_permission", "end_turn": "completed_turn"}[event.Trigger]
	metadata := map[string]any{"type": event.Trigger, "source_phase": phase}
	eventText := "Trigger: " + event.Trigger + "\nSource phase: " + phase
	if event.Trigger == "context_growth" {
		metadata["context_tokens"] = event.Tokens
		metadata["context_basis"] = "visible_input"
		metadata["context_estimated"] = event.Estimated
		metadata["model"] = event.Model
		metadata["source_active"] = source.Active
		countKind := ""
		if event.Estimated {
			countKind = " (estimate)"
		}
		eventText += fmt.Sprintf("\nVisible context at trigger (thinking excluded): %d tokens%s\nReviewed model: %s\nSource active: %t", event.Tokens, countKind, event.Model, source.Active)
	}
	eventJSON, _ := json.Marshal(metadata)
	todo, doing := taskReferences(source.Tasks)
	variables := map[string]string{"todo": todo, "doing": doing, "workspace": source.Workspace.Workspace, "event": eventText, "permission_request": permissionReference(event.Permission), "tool_call": "No pending tool call."}
	if event.Permission != nil {
		variables["tool_call"] = formatToolCall(event.Permission.ToolCall)
	}
	rows := transcriptMessages(source, value.IncludeReasoning)
	truncated := source.Truncated
	baseline, operationError := plugin.services.Sessions.Measure(operationContext, source.Workspace.ID, value.Model, value.ReasoningEffort, "", value.MaxOutputTokens)
	if operationError != nil {
		return "", harness.RequestBudget{}, false, operationError
	}
	measure := func(text string) (int, error) {
		budget, operationError := plugin.services.Sessions.Measure(operationContext, source.Workspace.ID, value.Model, value.ReasoningEffort, text, value.MaxOutputTokens)
		return max(0, budget.InputTokens-baseline.InputTokens), operationError
	}
	cache := map[string]string{}
	prompt, operationError := renderVariables(value.Prompt, func(name string) (string, error) {
		if text, found := variables[name]; found {
			return text, nil
		}
		if text, found := cache[name]; found {
			return text, nil
		}
		tokens, found, variableError := contextTokens(name, value.ContextTokens)
		if variableError != nil {
			return "", variableError
		}
		if !found {
			return "", fmt.Errorf("unknown watcher variable %q", name)
		}
		text, omitted, selectionError := selectTranscript(rows, tokens, measure)
		if selectionError != nil {
			return "", selectionError
		}
		truncated = truncated || omitted
		cache[name] = text
		return text, nil
	})
	if operationError != nil {
		return "", harness.RequestBudget{}, false, operationError
	}
	prompt = "<watcher_event>" + string(eventJSON) + "</watcher_event>\n" + prompt
	budget, operationError := plugin.services.Sessions.Measure(operationContext, source.Workspace.ID, value.Model, value.ReasoningEffort, prompt, value.MaxOutputTokens)
	if operationError == nil && budget.InputLimit > 0 && budget.InputTokens > budget.InputLimit {
		operationError = fmt.Errorf("watcher input uses %d tokens; input limit is %d", budget.InputTokens, budget.InputLimit)
	}
	return prompt, budget, truncated, operationError
}
func permissionID(request *atom.PermissionRequest) string {
	if request == nil {
		return ""
	}
	return request.ID
}
