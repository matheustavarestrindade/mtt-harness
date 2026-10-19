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

func transcriptMessages(source harness.ReviewSource, includeReasoning bool) []string {
	var rows []string
	for _, message := range source.Messages {
		message.ProviderState = nil
		message.Usage = nil
		if !includeReasoning {
			message.Reasoning = ""
		}
		data, _ := json.Marshal(struct {
			atom.Message
			InProgress bool `json:",omitempty"`
		}{message, message.ID == source.LiveMessageID})
		rows = append(rows, string(data))
	}
	return rows
}

// selectTranscript measures candidate text through the configured provider's
// counter/estimator. Complete message rows win; a too-large final row becomes an
// explicitly marked UTF-8 excerpt rather than silently violating the token limit.
func selectTranscript(rows []string, limit int, measure func(string) (int, error)) (string, bool, error) {
	if len(rows) == 0 {
		return "[]", false, nil
	}
	first, last := 0, len(rows)
	best := "[]"
	omitted := false
	for first <= last {
		middle := (first + last) / 2
		text := "[" + strings.Join(rows[middle:], ",\n") + "]"
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
	if best != "[]" {
		return best, omitted, nil
	}
	text := rows[len(rows)-1]
	lower, upper := 0, len(text)
	for lower <= upper {
		length := (lower + upper) / 2
		start := len(text) - length
		for start < len(text) && !utf8.RuneStart(text[start]) {
			start++
		}
		data, _ := json.Marshal(map[string]any{"truncated": true, "message_excerpt": text[start:]})
		tokens, operationError := measure(string(data))
		if operationError != nil {
			return "", false, operationError
		}
		if tokens <= limit {
			best = string(data)
			lower = length + 1
		} else {
			upper = length - 1
		}
	}
	return best, true, nil
}

func (plugin *Plugin) renderInput(operationContext context.Context, value definition, source harness.ReviewSource, event observation) (string, harness.RequestBudget, bool, error) {
	encode := func(value any) string { data, _ := json.Marshal(value); return string(data) }
	phase := map[string]string{"context_growth": "active_request", "permission_request": "pending_permission", "end_turn": "completed_turn"}[event.Trigger]
	variables := map[string]string{"todo": encode(source.Tasks.Todo), "doing": encode(source.Tasks.Doing), "workspace": encode(source.Workspace.Workspace), "event": encode(map[string]any{"type": event.Trigger, "source_phase": phase, "source_active": source.Active, "in_progress_message_id": source.LiveMessageID, "permission_id": permissionID(event.Permission), "context_tokens": event.Tokens, "model": event.Model}), "permission_request": encode(event.Permission), "tool_call": "null"}
	if event.Permission != nil {
		variables["tool_call"] = encode(event.Permission.ToolCall)
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
		if omitted {
			text = "Context excerpt: earlier content omitted.\n" + text
		}
		cache[name] = text
		return text, nil
	})
	if operationError != nil {
		return "", harness.RequestBudget{}, false, operationError
	}
	prompt = "<watcher_event>" + variables["event"] + "</watcher_event>\n" + prompt
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
