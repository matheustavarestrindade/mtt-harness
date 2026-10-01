package startprompt

import (
	"encoding/json"
	"fmt"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

// Values contains request-local context. Tool callbacks must use the runtime's
// authoritative registry; rendering does not discover or enable callable tools.
type Values struct {
	Session   atom.Session
	Workspace string
	Model     string
	ToolList  func() []atom.ToolReference
	ToolInfo  func(string) (atom.ToolSpec, error)
}

func renderVariable(variable string, values Values) (string, error) {
	switch variable {
	case "workspace":
		return values.Workspace, nil
	case "session_id":
		return string(values.Session.ID), nil
	case "instance_id":
		return values.Session.InstanceID, nil
	case "model":
		return values.Model, nil
	case "agent_depth":
		return strconv.Itoa(values.Session.Depth), nil
	case "os":
		return runtime.GOOS, nil
	case "arch":
		return runtime.GOARCH, nil
	case "tool_list":
		if values.ToolList == nil {
			return "", fmt.Errorf("tool registry is unavailable")
		}
		return renderToolList(values.ToolList()), nil
	default:
		if values.ToolInfo == nil {
			return "", fmt.Errorf("tool registry is unavailable")
		}
		specification, operationError := values.ToolInfo(strings.TrimSuffix(variable, "_info"))
		if operationError != nil {
			return "", operationError
		}
		return renderToolInfo(specification)
	}
}

func renderToolList(references []atom.ToolReference) string {
	references = slices.Clone(references)
	slices.SortFunc(references, func(first, second atom.ToolReference) int {
		return strings.Compare(first.Name, second.Name)
	})
	var lines []string
	for _, reference := range references {
		categories := slices.Clone(reference.Categories)
		slices.Sort(categories)
		lines = append(lines, "- "+reference.Name+" ["+strings.Join(categories, ", ")+"]")
	}
	return strings.Join(lines, "\n")
}

func renderToolInfo(specification atom.ToolSpec) (string, error) {
	// Only the callable contract is exposed. Search-only usage documents and
	// embeddings are not part of ToolSpec and must not enter the prompt.
	definition := struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Categories  []string        `json:"categories"`
		InputSchema json.RawMessage `json:"input_schema"`
	}{specification.Name, specification.Description, specification.Categories, specification.InputSchema.JSON}
	encoded, operationError := json.MarshalIndent(definition, "", "  ")
	if operationError != nil {
		return "", fmt.Errorf("encode tool %q: %w", specification.Name, operationError)
	}
	return string(encoded), nil
}
