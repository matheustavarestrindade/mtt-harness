package tools

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (FileActions) Operations() []string {
	return []string{"read", "list", "glob", "write", "replace", "append", "prepend", "delete"}
}

func (fileTool FileActions) ValidateOperations(call atom.ToolCall, allowed []string) error {
	var input struct {
		Actions []struct {
			Operation string `json:"op"`
		} `json:"actions"`
		Return struct {
			Type string `json:"type"`
		} `json:"return"`
	}
	if operationError := json.Unmarshal(call.Input, &input); operationError != nil {
		return operationError
	}
	for _, action := range input.Actions {
		if !slices.Contains(allowed, action.Operation) {
			return fmt.Errorf("file_actions operation %q is not enabled for this session", action.Operation)
		}
	}
	inspection := input.Return.Type
	if inspection == "diff" {
		inspection = "read"
	}
	if slices.Contains([]string{"read", "list", "glob"}, inspection) && !slices.Contains(allowed, inspection) {
		return fmt.Errorf("file_actions return %q requires operation %q", input.Return.Type, inspection)
	}
	return nil
}

func (fileTool FileActions) WithOperations(allowed []string) (atom.ToolSpec, error) {
	if len(allowed) == 0 {
		return atom.ToolSpec{}, fmt.Errorf("file_actions requires an operation selection")
	}
	for _, operation := range allowed {
		if !slices.Contains(fileTool.Operations(), operation) {
			return atom.ToolSpec{}, fmt.Errorf("unknown file_actions operation %q", operation)
		}
	}
	var definition map[string]any
	if operationError := json.Unmarshal(fileTool.InputSchema().JSON, &definition); operationError != nil {
		return atom.ToolSpec{}, operationError
	}
	properties := definition["properties"].(map[string]any)
	actions := properties["actions"].(map[string]any)["items"].(map[string]any)
	actions["properties"].(map[string]any)["op"].(map[string]any)["enum"] = allowed
	returns := []string{"summary"}
	for _, kind := range []string{"diff", "read", "list", "glob"} {
		required := kind
		if kind == "diff" {
			required = "read"
		}
		if slices.Contains(allowed, required) {
			returns = append(returns, kind)
		}
	}
	properties["return"].(map[string]any)["properties"].(map[string]any)["type"].(map[string]any)["enum"] = returns
	// Examples must describe enabled operations too; descriptions/defaults remain
	// the authoritative tool documentation, followed by the exact capability set.
	var examples []any
	for _, example := range definition["examples"].([]any) {
		data, operationError := json.Marshal(example)
		if operationError != nil {
			return atom.ToolSpec{}, operationError
		}
		if fileTool.ValidateOperations(atom.ToolCall{Input: data}, allowed) == nil {
			examples = append(examples, example)
		}
	}
	definition["examples"] = examples
	data, operationError := json.Marshal(definition)
	if operationError != nil {
		return atom.ToolSpec{}, operationError
	}
	return atom.ToolSpec{Name: fileTool.Name(), Description: fileTool.Description() + " This session enables only these action operations: " + strings.Join(allowed, ", ") + ". Every action in a chain must be enabled.", Categories: fileTool.Categories(), InputSchema: atom.Schema{JSON: data}}, nil
}
