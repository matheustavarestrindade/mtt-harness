package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func supportedFormat(path string) bool {
	if path == "" {
		return false
	}
	switch filepath.Ext(path) {
	case ".go", ".json":
		return true
	case ".ts", ".js", ".mjs", ".svelte", ".css", ".html":
		return strings.HasPrefix(path, "ui/")
	default:
		return false
	}
}

func formatContent(repositoryPath, relativePath string, content []byte) ([]byte, error) {
	if filepath.Ext(relativePath) == ".go" {
		return format.Source(content)
	}
	if !strings.HasPrefix(relativePath, "ui/") {
		var formattedJSON bytes.Buffer
		if operationError := json.Indent(&formattedJSON, bytes.TrimSpace(content), "", "  "); operationError != nil {
			return nil, operationError
		}
		return append(formattedJSON.Bytes(), '\n'), nil
	}
	uiPath := filepath.Join(repositoryPath, "ui")
	prettierPath := filepath.Join(uiPath, "node_modules", "prettier", "bin", "prettier.cjs")
	if _, operationError := os.Stat(prettierPath); operationError != nil {
		return nil, fmt.Errorf("UI formatter unavailable; run npm ci inside ui/: %w", operationError)
	}
	command := exec.Command("node", prettierPath, "--config", filepath.Join(uiPath, ".prettierrc.json"), "--stdin-filepath", filepath.Join(repositoryPath, relativePath))
	command.Dir = uiPath
	command.Stdin = bytes.NewReader(content)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	formattedContent, operationError := command.Output()
	if operationError != nil {
		return nil, fmt.Errorf("prettier: %w: %s", operationError, strings.TrimSpace(stderr.String()))
	}
	return formattedContent, nil
}
