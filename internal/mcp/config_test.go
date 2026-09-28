package mcp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestLoadFile(test *testing.T) {
	path := filepath.Join(test.TempDir(), "mcp.json")
	data := `{
		"servers": [
			{"name": "local", "command": "npx", "args": ["-y", "server"], "enabled": true},
			{"name": "remote", "url": "https://example.com/mcp", "headers": {"Authorization": "Bearer x"}, "enabled": false}
		]
	}`
	testutil.RequireNoError(test, os.WriteFile(path, []byte(data), 0o644))

	servers, operationError := LoadFile(path)
	testutil.RequireNoError(test, operationError)

	if len(servers) != 2 {
		test.Fatalf("servers = %d", len(servers))
	}
	if servers[0].Name != "local" || servers[0].Command != "npx" || !servers[0].Enabled {
		test.Fatalf("local server = %+v", servers[0])
	}
	if servers[1].URL != "https://example.com/mcp" || servers[1].Headers["Authorization"] != "Bearer x" {
		test.Fatalf("remote server = %+v", servers[1])
	}
}
