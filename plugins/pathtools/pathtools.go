package pathtools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type PathGuard struct {
	WorkspaceOf func(instanceID string) string
}

func (pathGuard *PathGuard) Name() string {
	return "pathtools"
}
func (pathGuard *PathGuard) Version() string {
	return "0.1.0"
}

func (pathGuard *PathGuard) Setup(harnessRuntime *harness.Harness) error {
	harness.Decide(harnessRuntime, atom.StageToolInput, func(operationContext context.Context, call atom.ToolCall) (atom.Verdict, error) {
		var input struct {
			Path string `json:"path"`
		}
		if operationError := json.Unmarshal(call.Input, &input); operationError != nil {
			return atom.Verdict{}, operationError
		}
		paths := []string{}
		if input.Path != "" {
			paths = append(paths, input.Path)
		}
		resolvePath := harness.WorkspacePath
		if call.Name == "file_actions" {
			var fileInput struct {
				Paths   []string `json:"paths"`
				Actions []struct {
					Operation string `json:"op"`
				} `json:"actions"`
			}
			if operationError := json.Unmarshal(call.Input, &fileInput); operationError != nil {
				return atom.Verdict{}, operationError
			}
			paths = append(paths, fileInput.Paths...)
			for _, action := range fileInput.Actions {
				if action.Operation == "delete" {
					resolvePath = harness.WorkspaceEntryPath
					break
				}
			}
		}
		if len(paths) == 0 {
			return atom.Verdict{Kind: atom.VerdictAllow}, nil
		}
		workspace, found := harness.WorkspaceFrom(operationContext)
		if !found && pathGuard.WorkspaceOf != nil {
			session, _ := harness.SessionFrom(operationContext)
			workspace = pathGuard.WorkspaceOf(session.InstanceID)
			operationContext = harness.WithWorkspace(operationContext, workspace)
		}
		if workspace == "" {
			return atom.Verdict{}, fmt.Errorf("path check has no instance workspace")
		}
		root, operationError := harness.ResolvePath(workspace)
		if operationError != nil {
			return atom.Verdict{}, operationError
		}
		var outside []string
		seen := map[string]bool{}
		for _, path := range paths {
			target, operationError := resolvePath(operationContext, path)
			if operationError != nil {
				return atom.Verdict{}, operationError
			}
			relative, operationError := filepath.Rel(root, target)
			if operationError != nil {
				return atom.Verdict{}, operationError
			}
			if relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative) {
				continue
			}
			if !seen[target] {
				outside = append(outside, target)
				seen[target] = true
			}
		}
		if len(outside) == 0 {
			return atom.Verdict{Kind: atom.VerdictAllow}, nil
		}
		if len(outside) == 1 {
			return atom.Verdict{Kind: atom.VerdictAsk, Target: outside[0], Why: "the path is not in the workspace"}, nil
		}
		encoded, operationError := json.Marshal(outside)
		if operationError != nil {
			return atom.Verdict{}, operationError
		}
		return atom.Verdict{Kind: atom.VerdictAsk, Target: string(encoded), Why: "the paths are not in the workspace"}, nil
	})
	return nil
}
