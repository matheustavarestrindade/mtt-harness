package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/startprompt"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
	"github.com/matheustavarestrindade/mtt-harness/internal/tools"
)

func TestStartupPromptConfigurationAndFlagPrecedence(test *testing.T) {
	configurationPath := filepath.Join(test.TempDir(), "mtt.json")
	testutil.RequireNoError(test, os.WriteFile(configurationPath, []byte(`{"start_prompt_file":"configured.md"}`), 0600))
	configuration, operationError := parseArguments([]string{"--config", configurationPath})
	testutil.RequireNoError(test, operationError)
	if configuration.StartPromptFile != "configured.md" {
		test.Fatalf("prompt file configuration ignored: %+v", configuration)
	}
	configuration, operationError = parseArguments([]string{"--config", configurationPath, "--start-prompt-file", "override.md"})
	testutil.RequireNoError(test, operationError)
	if configuration.StartPromptFile != "override.md" {
		test.Fatal("prompt flag did not override the bootstrap file")
	}
	if operationError := runCommandLine([]string{"--config", configurationPath, "--start-prompt-file", filepath.Join(test.TempDir(), "absent.md")}); operationError == nil || !strings.Contains(operationError.Error(), "load startup prompt") {
		test.Fatalf("missing startup prompt was not reported before starting services: %v", operationError)
	}
}

func TestShippedStartupTemplateUsesSupportedVariables(test *testing.T) {
	template, operationError := startprompt.Load(filepath.Join("..", "..", "start_prompt.md"))
	testutil.RequireNoError(test, operationError)
	searchTool := tools.NewSearch(nil)
	rendered, operationError := template.Render(context.Background(), startprompt.Values{
		ToolList: func() []atom.ToolReference {
			return []atom.ToolReference{{Name: searchTool.Name(), Categories: searchTool.Categories()}}
		},
		ToolInfo: func(name string) (atom.ToolSpec, error) {
			if name != searchTool.Name() {
				test.Fatalf("shipped template references an unexpected tool: %q", name)
			}
			return atom.ToolSpec{
				Name: searchTool.Name(), Description: searchTool.Description(),
				Categories: searchTool.Categories(), InputSchema: searchTool.InputSchema(),
			}, nil
		},
	})
	testutil.RequireNoError(test, operationError)
	if strings.Contains(rendered, "{search_tool_info}") || !strings.Contains(rendered, `"name": "search_tool"`) {
		test.Fatal("shipped template did not include the search tool definition")
	}
}
