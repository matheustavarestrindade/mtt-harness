package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/registry"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestReadLineNumberExperimentReachesRegisteredTool(test *testing.T) {
	for _, scenario := range []struct{ value, expected string }{
		{"", "second\n"}, {"false", "second\n"}, {"0", "second\n"}, {"true", "2: second\n"}, {"1", "2: second\n"},
	} {
		test.Run("value="+scenario.value, func(test *testing.T) {
			test.Setenv("MTT_READ_LINE_NUMBERS", scenario.value)
			workspace := test.TempDir()
			testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, "file.txt"), []byte("first\nsecond\nthird"), 0o600))
			harnessRuntime := harness.New()
			toolRegistry := registry.New(harnessRuntime, nil)
			attachTools(toolRegistry, nil, nil)
			readTool, found := toolRegistry.Get("file_actions")
			if !found {
				test.Fatal("file_actions was not registered")
			}
			arguments, _ := json.Marshal(map[string]any{"path": "file.txt", "actions": []any{map[string]any{"op": "read", "start_line": 2, "end_line": 2}}})
			result, operationError := readTool.Run(harness.WithWorkspace(context.Background(), workspace), atom.ToolCall{Input: arguments})
			testutil.RequireNoError(test, operationError)
			if result.Text() != scenario.expected {
				test.Fatalf("output = %q, want %q", result.Text(), scenario.expected)
			}
			// A benchmark run keeps its variant after registration, even if the
			// process environment is subsequently changed by another component.
			test.Setenv("MTT_READ_LINE_NUMBERS", "false")
			result, operationError = readTool.Run(harness.WithWorkspace(context.Background(), workspace), atom.ToolCall{Input: arguments})
			testutil.RequireNoError(test, operationError)
			if result.Text() != scenario.expected {
				test.Fatal("read experiment changed after registration")
			}
			if scenario.value == "true" || scenario.value == "1" {
				if !strings.Contains(readTool.Description(), "display labels") {
					test.Fatal("numbered mode lacks instructions for the model")
				}
			}
			for _, name := range []string{"read", "write", "replace"} {
				if _, found := toolRegistry.Get(name); found {
					test.Fatalf("retired file tool %s was registered", name)
				}
			}
		})
	}
}

func TestInvalidReadExperimentFailsConfiguration(test *testing.T) {
	test.Setenv("MTT_READ_LINE_NUMBERS", "perhaps")
	if _, operationError := readLineNumbersExperiment(); operationError == nil {
		test.Fatal("invalid A/B variant was accepted")
	}
}
