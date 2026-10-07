package embedding

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ContextConfiguration selects the local memory encoder from providers.json.
// Workspace behavior and worker models remain database-owned plugin settings.
type ContextConfiguration struct {
	Backend        string `json:"backend"`
	ModelDirectory string `json:"model_directory"`
	ModelFile      string `json:"model_file"`
	RuntimeLibrary string `json:"runtime_library"`
	ChunkTokens    int    `json:"chunk_tokens"`
	Threads        int    `json:"threads"`
}

func LoadContextConfiguration(path string) (ContextConfiguration, error) {
	configuration := ContextConfiguration{Backend: "disabled"}
	data, operationError := os.ReadFile(path)
	if errors.Is(operationError, os.ErrNotExist) {
		return configuration, nil
	}
	if operationError != nil {
		return configuration, operationError
	}
	var document struct {
		ContextEmbeddings json.RawMessage `json:"context_embeddings"`
	}
	if operationError := json.Unmarshal(data, &document); operationError != nil {
		return configuration, operationError
	}
	if len(document.ContextEmbeddings) == 0 {
		return configuration, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(document.ContextEmbeddings))
	decoder.DisallowUnknownFields()
	if operationError := decoder.Decode(&configuration); operationError != nil {
		return configuration, fmt.Errorf("context_embeddings: %w", operationError)
	}
	switch configuration.Backend {
	case "disabled":
		return configuration, nil
	case "minilm":
	case "embeddinggemma2":
		if configuration.ModelFile == "" || filepath.IsAbs(configuration.ModelFile) || !filepath.IsLocal(configuration.ModelFile) {
			return configuration, fmt.Errorf("context_embeddings.model_file must be a local relative file path")
		}
		if configuration.RuntimeLibrary == "" {
			return configuration, fmt.Errorf("context_embeddings.runtime_library is empty")
		}
		if configuration.ChunkTokens == 0 {
			configuration.ChunkTokens = 1024
		}
		if configuration.Threads == 0 {
			configuration.Threads = 2
		}
		if configuration.ChunkTokens < 32 || configuration.ChunkTokens > 8192 {
			return configuration, fmt.Errorf("context_embeddings.chunk_tokens must be between 32 and 8192")
		}
		if configuration.Threads < 1 || configuration.Threads > 64 {
			return configuration, fmt.Errorf("context_embeddings.threads must be between 1 and 64")
		}
	default:
		return configuration, fmt.Errorf("context_embeddings.backend %q is not supported", configuration.Backend)
	}
	if configuration.ModelDirectory == "" {
		return configuration, fmt.Errorf("context_embeddings.model_directory is empty")
	}
	for _, value := range []*string{&configuration.ModelDirectory, &configuration.RuntimeLibrary} {
		if *value == "" || filepath.IsAbs(*value) {
			continue
		}
		absolute, operationError := filepath.Abs(filepath.Join(filepath.Dir(path), *value))
		if operationError != nil {
			return configuration, operationError
		}
		*value = absolute
	}
	return configuration, nil
}
