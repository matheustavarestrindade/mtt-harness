package bridge

import (
	"encoding/json"
	"os"
)

type FileEntry struct {
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Enabled bool     `json:"enabled"`
}

type fileConfig struct {
	Plugins []FileEntry `json:"plugins"`
}

func LoadFile(path string) ([]FileEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file fileConfig
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	return file.Plugins, nil
}
