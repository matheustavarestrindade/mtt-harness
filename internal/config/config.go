package config

import (
	"encoding/json"
	"os"
)

type File struct {
	Port                    int    `json:"port"`
	DatabaseURL             string `json:"database_url"`
	ProvidersFile           string `json:"providers_file"`
	MCPFile                 string `json:"mcp_file"`
	StartPromptFile         string `json:"start_prompt_file"`
	SpacedRepetitionPrompts string `json:"spaced_repetition_prompts"`
	APIToken                string `json:"api_token"`
	TestProvider            bool   `json:"test_provider"`
}

func Default() File {
	return File{
		Port:                    8080,
		ProvidersFile:           "providers.json",
		MCPFile:                 "mcp.json",
		StartPromptFile:         "start_prompt.md",
		SpacedRepetitionPrompts: "plugins/spaced_repetition/prompts",
	}
}

func LoadOrDefault(path string) (File, error) {
	bootstrap := Default()
	data, operationError := os.ReadFile(path)
	if os.IsNotExist(operationError) {
		return bootstrap, nil
	}
	if operationError != nil {
		return bootstrap, operationError
	}
	if operationError := json.Unmarshal(data, &bootstrap); operationError != nil {
		return bootstrap, operationError
	}
	return bootstrap, nil
}
