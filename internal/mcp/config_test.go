package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	data := `{
		"servers": [
			{"name": "local", "command": "npx", "args": ["-y", "server"], "enabled": true},
			{"name": "remote", "url": "https://example.com/mcp", "headers": {"Authorization": "Bearer x"}, "enabled": false}
		]
	}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	servers, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 {
		t.Fatalf("servers = %d", len(servers))
	}
	if servers[0].Name != "local" || servers[0].Command != "npx" || !servers[0].Enabled {
		t.Fatalf("local server = %+v", servers[0])
	}
	if servers[1].URL != "https://example.com/mcp" || servers[1].Headers["Authorization"] != "Bearer x" {
		t.Fatalf("remote server = %+v", servers[1])
	}
}
