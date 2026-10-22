package watcher

import (
	"encoding/json"
	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func integerSetting(description string, minimum, maximum, defaultValue int) map[string]any {
	return map[string]any{"type": "integer", "oneOf": []any{map[string]any{"const": 0}, map[string]any{"minimum": minimum, "maximum": maximum}}, "default": defaultValue, "description": description + " Omitted or zero uses the default."}
}
func configurationSchema(tools []harness.WorkerTool) atom.Schema {
	defaults := definition{ContextTokens: 32768, IntervalTokens: 32768, TimeoutMS: 120000, MaxRounds: 12, MaxOutputTokens: 2048, MaxContinuations: 3, Prompt: defaultPrompt, Triggers: []string{"end_turn"}}
	applyDefaults(&defaults)
	watcher := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"id", "name", "triggers"}, "properties": map[string]any{
		"id":                     map[string]any{"type": "string", "minLength": 1, "maxLength": 100, "description": "Stable watcher ID, unique in this configuration; changing it creates a distinct schedule and usage identity."},
		"name":                   map[string]any{"type": "string", "minLength": 1, "maxLength": 200, "description": "Display name, 1-200 UTF-8 bytes."},
		"enabled":                map[string]any{"type": "boolean", "default": false, "description": "Enable this definition when the workspace plugin is also enabled."},
		"model":                  map[string]any{"type": "string", "description": "Provider-qualified workspace model ID. Required for an enabled watcher; there is no worker-model default."},
		"reasoning_effort":       map[string]any{"type": "string", "default": "", "description": "Reasoning effort supported by the selected model; empty uses its default."},
		"prompt":                 map[string]any{"type": "string", "maxLength": 65536, "default": defaultPrompt, "description": "UTF-8 template, at most 65536 bytes. todo/doing/event/workspace/permission_request/tool_call/last_context and last_N[k]_context variables expand once. Double braces escape a variable. k means 1024 tokens."},
		"triggers":               map[string]any{"type": "array", "minItems": 1, "uniqueItems": true, "items": map[string]any{"type": "string", "enum": []string{"context_growth", "permission_request", "end_turn"}}, "description": "Select context growth, pending permission requests, normal main-turn completion, or a combination. Watcher executions never trigger reviews."},
		"interval_tokens":        integerSetting("Visible-context growth interval in tokens. Default 32768; 256-1048576. Counts selected messages, tools and media with the model's counter/estimator, excluding exposed thinking, private reasoning continuation and aggregate usage anchors. Thinking alone cannot advance the interval, even when include_reasoning is true. This is not billed usage or full model-context occupancy. Compaction/model/count-basis changes rebase the interval.", 256, 1048576, 32768),
		"context_tokens":         integerSetting("Default upper conversation-token budget for last_context, measured with the selected model's counter or estimator after the include_reasoning option. Whole-message selection can use less than the limit. Fixed instructions and tool definitions are outside this conversation budget. Explicit last_N[k]_context variables use their own window. Default 32768; 256-1048576. Source acquisition also caps at 1024 messages and 4 MiB of public text.", 256, 1048576, 32768),
		"include_reasoning":      map[string]any{"type": "boolean", "default": false, "description": "Include provider-exposed thinking text in the reviewer's reference context only. It never counts toward the context-growth trigger. Private provider continuation data is never included."},
		"permission_tools":       map[string]any{"type": "array", "default": []any{}, "uniqueItems": true, "items": map[string]any{"type": "string"}, "description": "Optional exact tool names for permission events. Empty matches all tools."},
		"outside_workspace_only": map[string]any{"type": "boolean", "default": false, "description": "For permission events, require an absolute resolved target outside the workspace. Does not infer shell filesystem behavior from command text."},
		"tools":                  map[string]any{"type": "array", "default": []any{}, "description": "Explicit pre-authorized capabilities. Discovery and execution are restricted to this selection; selected tools bypass the permission pipeline. Decision/discovery lifecycle tools are intrinsic. File actions require operation choices.", "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"name"}, "properties": map[string]any{"name": map[string]any{"type": "string", "description": "Exact registered tool name."}, "operations": map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string"}, "description": "Operations for an operation-aware tool, including any read/list/glob operation needed by its return selection. Omit for tools with no operation contract."}}}},
		"timeout_ms":             integerSetting("Maximum duration of one watcher run, in milliseconds. Default 120000; 1000-900000. It does not limit a user's permission wait.", 1000, 900000, 120000),
		"max_rounds":             integerSetting("Maximum model/tool rounds in one watcher session. Default 12; 1-100.", 1, 100, 12),
		"max_output_tokens":      integerSetting("Output-token reservation per watcher model request. Default 2048; 128-65536. The complete rendered prompt and tool definitions must fit the model.", 128, 65536, 2048),
		"max_continuations":      integerSetting("Maximum automatic context/completion interventions per watcher and user-instruction chain. Default 3; 1-100. New turns alone do not reset it; new user input does.", 1, 100, 3),
	}}
	data, _ := json.Marshal(map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"enabled": map[string]any{"type": "boolean", "default": false, "description": "Enable workspace watchers. OFF by default. Disabling cancels reviews and retains definitions and activity."}, "watchers": map[string]any{"type": "array", "default": []any{}, "items": watcher, "description": "Watcher definitions. A workspace array replaces the global array; there is no fixed small count limit."}}, "x-tools": tools, "x-default-definition": defaults, "x-triggers": []string{"context_growth", "permission_request", "end_turn"}, "x-variables": []string{"todo", "doing", "event", "workspace", "permission_request", "tool_call", "last_context", "last_32k_context"}})
	return atom.Schema{JSON: data}
}
