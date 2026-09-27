package config

import (
	"encoding/json"
	"os"
)

type File struct {
	Port          int    `json:"port"`
	DatabaseURL   string `json:"database_url"`
	ProvidersFile string `json:"providers_file"`
	PluginsFile   string `json:"plugins_file"`
	APIToken      string `json:"api_token"`
	TestProvider  bool   `json:"test_provider"`
}

func Default() File {
	return File{
		Port:          8080,
		ProvidersFile: "providers.json",
		PluginsFile:   "plugins.json",
	}
}

func LoadOrDefault(path string) (File, error) {
	bootstrap := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return bootstrap, nil
		}
		return bootstrap, err
	}
	if err := json.Unmarshal(data, &bootstrap); err != nil {
		return bootstrap, err
	}
	return bootstrap, nil
}
