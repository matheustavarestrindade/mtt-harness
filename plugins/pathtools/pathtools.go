package pathtools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type PathGuard struct {
	Workspace string
}

func (p *PathGuard) Name() string {
	return "pathtools"
}

func (p *PathGuard) Version() string {
	return "0.1.0"
}

func (p *PathGuard) Setup(h *harness.Harness) error {
	harness.Decide(h, atom.StageToolInput, func(ctx context.Context, call atom.ToolCall) (atom.Verdict, error) {
		var input struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(call.Input, &input); err != nil || input.Path == "" {
			return atom.Verdict{Kind: atom.VerdictAllow}, nil
		}
		target := resolve(input.Path)
		workspace := resolve(p.Workspace)
		if target == workspace || strings.HasPrefix(target, workspace+string(filepath.Separator)) {
			return atom.Verdict{Kind: atom.VerdictAllow}, nil
		}
		return atom.Verdict{
			Kind:   atom.VerdictAsk,
			Target: target,
			Why:    "the path is not in the workspace",
		}, nil
	})
	return nil
}

func resolve(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return absolute
	}
	return resolved
}
