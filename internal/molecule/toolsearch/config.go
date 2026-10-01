package toolsearch

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

type Mode string

const (
	ModeAuto     Mode = "auto"
	ModeSemantic Mode = "semantic"
	ModeLexical  Mode = "lexical"
)

// Configuration is the tool_search section of providers.json. Local model
// assets and mode selection are independent of generation-provider credentials.
type Configuration struct {
	Mode                      Mode    `json:"mode"`
	ModelDirectory            string  `json:"model_directory"`
	SemanticMinimumSimilarity float64 `json:"semantic_minimum_similarity"`
	LexicalMinimumSimilarity  float64 `json:"lexical_minimum_similarity"`
}

func DefaultConfiguration() Configuration {
	return Configuration{Mode: ModeAuto, ModelDirectory: "/opt/mtt/models/all-MiniLM-L6-v2", SemanticMinimumSimilarity: 0.3, LexicalMinimumSimilarity: 0.01}
}

func LoadConfiguration(path string) (Configuration, error) {
	file := struct {
		ToolSearch Configuration `json:"tool_search"`
	}{ToolSearch: DefaultConfiguration()}
	data, operationError := os.ReadFile(path)
	if operationError != nil && !errors.Is(operationError, os.ErrNotExist) {
		return Configuration{}, operationError
	}
	if operationError == nil {
		if operationError := json.Unmarshal(data, &file); operationError != nil {
			return Configuration{}, operationError
		}
	}
	configuration := file.ToolSearch
	if configuration.Mode != ModeAuto && configuration.Mode != ModeSemantic && configuration.Mode != ModeLexical {
		return Configuration{}, fmt.Errorf("tool_search.mode must be auto, semantic, or lexical")
	}
	if configuration.Mode != ModeLexical && configuration.ModelDirectory == "" {
		return Configuration{}, fmt.Errorf("tool_search.model_directory is required for semantic search")
	}
	if !validMinimum(configuration.SemanticMinimumSimilarity) || !validMinimum(configuration.LexicalMinimumSimilarity) {
		return Configuration{}, fmt.Errorf("tool search similarity minimums must be finite values between 0 and 1")
	}
	if configuration.ModelDirectory != "" && !filepath.IsAbs(configuration.ModelDirectory) {
		absolutePath, operationError := filepath.Abs(filepath.Join(filepath.Dir(path), configuration.ModelDirectory))
		if operationError != nil {
			return Configuration{}, operationError
		}
		configuration.ModelDirectory = absolutePath
	}
	return configuration, nil
}

func validMinimum(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}
