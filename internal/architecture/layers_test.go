package architecture_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// Go's internal rule protects the module from outside imports. These tests
// enforce the stricter in-module boundaries promised by the harness design.
func TestProjectLayerBoundaries(test *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	const module = "github.com/matheustavarestrindade/mtt-harness/"
	for _, layer := range []string{"atom", "harness", "internal/molecule", "plugins"} {
		operationError := filepath.WalkDir(filepath.Join(root, layer), func(path string, entry os.DirEntry, walkError error) error {
			if walkError != nil {
				return walkError
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, parseError := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if parseError != nil {
				return parseError
			}
			for _, dependency := range file.Imports {
				importPath, parseError := strconv.Unquote(dependency.Path.Value)
				if parseError != nil {
					return parseError
				}
				if !strings.HasPrefix(importPath, module) {
					continue
				}
				projectPath := strings.TrimPrefix(importPath, module)
				invalid := layer == "atom" || (layer == "harness" && projectPath != "atom") || (layer == "plugins" && strings.HasPrefix(projectPath, "internal/")) || (layer == "internal/molecule" && strings.HasPrefix(projectPath, "internal/organism/"))
				if invalid {
					test.Errorf("%s must not import %s", path, importPath)
				}
			}
			return nil
		})
		if operationError != nil {
			test.Fatal(operationError)
		}
	}
}
