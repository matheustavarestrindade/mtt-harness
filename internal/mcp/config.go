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
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file fileConfig
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	return file.Servers, nil
}
