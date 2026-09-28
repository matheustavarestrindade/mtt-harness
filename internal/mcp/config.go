package mcp

import (
	"encoding/json"
	"os"
)

type ServerSpec struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Enabled bool              `json:"enabled"`
}

type fileConfig struct {
	Servers []ServerSpec `json:"servers"`
}

func LoadFile(path string) ([]ServerSpec, error) {
	data, operationError := os.ReadFile(path)
	if operationError != nil {
		return nil, operationError
	}
	var file fileConfig
	if operationError := json.Unmarshal(data, &file); operationError != nil {
		return nil, operationError
	}
	return file.Servers, nil
}
