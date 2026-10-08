package main

import (
	"flag"

	"github.com/matheustavarestrindade/mtt-harness/internal/config"
	"github.com/matheustavarestrindade/mtt-harness/internal/operation"
)

func parseArguments(arguments []string) (config.File, error) {
	flags := flag.NewFlagSet("mtt", flag.ContinueOnError)
	configurationPath := flags.String("config", "mtt.json", "the bootstrap config file")
	port := flags.Int("port", 0, "the API port")
	databaseURL := flags.String("database-url", "", "the Postgres URL")
	providersFile := flags.String("providers-file", "", "the providers file")
	mcpFile := flags.String("mcp-file", "", "the MCP server file")
	startPromptFile := flags.String("start-prompt-file", "", "the startup prompt template file (default start_prompt.md)")
	spacedPrompts := flags.String("spaced-repetition-prompts", "", "directory containing low.md, medium.md, and high.md reminder templates")
	testProvider := flags.Bool("test-provider", false, "use the test provider")
	if operationError := flags.Parse(arguments); operationError != nil {
		return config.File{}, operation.WrapError(operationError, "parse arguments")
	}
	configuration, operationError := config.LoadOrDefault(*configurationPath)
	if operationError != nil {
		return config.File{}, operation.WrapError(operationError, "load bootstrap file")
	}
	if *port != 0 {
		configuration.Port = *port
	}
	if *databaseURL != "" {
		configuration.DatabaseURL = *databaseURL
	}
	if *providersFile != "" {
		configuration.ProvidersFile = *providersFile
	}
	if *mcpFile != "" {
		configuration.MCPFile = *mcpFile
	}
	if *startPromptFile != "" {
		configuration.StartPromptFile = *startPromptFile
	}
	if *spacedPrompts != "" {
		configuration.SpacedRepetitionPrompts = *spacedPrompts
	}
	flags.Visit(func(option *flag.Flag) {
		if option.Name == "test-provider" {
			configuration.TestProvider = *testProvider
		}
	})
	return configuration, nil
}
