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
	workspace := filepath.Join(test.TempDir(), "project with spaces")
	searchTool := tools.NewSearch(nil)
	values := startprompt.Values{
		Workspace: workspace,
		ToolList: func() []atom.ToolReference {
			return []atom.ToolReference{{Name: searchTool.Name(), Categories: searchTool.Categories()}}
		},
		ToolInfo: func(name string) (atom.ToolSpec, error) {
			test.Fatalf("default prompt expanded a full tool definition: %s", name)
			return atom.ToolSpec{}, nil
		},
	}
	for _, templatePath := range []string{
		"start_prompt.md",
		"plugins/spaced_repetition/prompts/low.md",
		"plugins/spaced_repetition/prompts/medium.md",
		"plugins/spaced_repetition/prompts/high.md",
	} {
		test.Run(templatePath, func(test *testing.T) {
			template, operationError := startprompt.Load(filepath.Join("..", "..", filepath.FromSlash(templatePath)))
			testutil.RequireNoError(test, operationError)
			rendered, operationError := template.Render(context.Background(), values)
			testutil.RequireNoError(test, operationError)
			if !strings.Contains(rendered, workspace) || strings.Contains(rendered, "{workspace}") {
				test.Fatal("shipped instructions did not render the session workspace")
			}
			if strings.Contains(rendered, "{search_tool_info}") || strings.Contains(rendered, "input_schema") {
				test.Fatal("shipped instructions did not keep tool metadata in the callable definitions")
			}
			if templatePath == "start_prompt.md" && !strings.Contains(rendered, "- search_tool") {
				test.Fatal("startup template omitted the discovery catalog")
			}
		})
	}
}
