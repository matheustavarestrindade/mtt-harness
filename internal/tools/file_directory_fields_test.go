package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/harness"
	inputSchemaValidation "github.com/matheustavarestrindade/mtt-harness/internal/molecule/schema"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

type nameOnlyDirectoryEntry struct{ metadataReads int }

func (*nameOnlyDirectoryEntry) Name() string      { return "name.txt" }
func (*nameOnlyDirectoryEntry) IsDir() bool       { return false }
func (*nameOnlyDirectoryEntry) Type() os.FileMode { return 0 }
func (entry *nameOnlyDirectoryEntry) Info() (os.FileInfo, error) {
	entry.metadataReads++
	return nil, os.ErrPermission
}

func TestCompactListingDoesNotFetchUnusedMetadata(test *testing.T) {
	entry := &nameOnlyDirectoryEntry{}
	text, operationError := formatDirectoryEntry(entry, nil)
	testutil.RequireNoError(test, operationError)
	if text != "F \"name.txt\"\n" || entry.metadataReads != 0 {
		test.Fatalf("default fetched or emitted metadata: %q, %d", text, entry.metadataReads)
	}
}

func TestDirectoryMetadataIsExplicitAndAvailableInReturn(test *testing.T) {
	workspace := test.TempDir()
	path := filepath.Join(workspace, "name.txt")
	testutil.RequireNoError(test, os.WriteFile(path, []byte("data"), 0o640))
	testutil.RequireNoError(test, os.Chmod(path, 0o640))
	modified := time.Date(2025, 2, 3, 4, 5, 6, 0, time.UTC)
	testutil.RequireNoError(test, os.Chtimes(path, modified, modified))
	information, operationError := os.Stat(path)
	testutil.RequireNoError(test, operationError)
	fields := []string{"permissions", "size", "modified", "owner", "group"}
	wanted := fmt.Sprintf("F \"name.txt\"\tpermissions=0640\tsize=4\tmodified=%s", information.ModTime().UTC().Format(time.RFC3339Nano))
	owner, group, available := fileOwnerIDs(information)
	if available {
		wanted += fmt.Sprintf("\towner=%d\tgroup=%d\n", owner, group)
	} else {
		wanted += "\towner=?\tgroup=?\n"
	}
	for _, useReturn := range []bool{false, true} {
		action := map[string]any{"op": "list", "fields": fields}
		input := map[string]any{"path": ".", "actions": []any{action}}
		if useReturn {
			action["fields"] = []string{}
			input["return"] = map[string]any{"type": "list", "fields": fields}
		}
		call := fileCall(test, input)
		fileTool := NewFileActions(false)
		testutil.RequireNoError(test, inputSchemaValidation.Validate(fileTool.InputSchema().JSON, call.Input))
		result, operationError := fileTool.Run(harness.WithWorkspace(context.Background(), workspace), call)
		testutil.RequireNoError(test, operationError)
		if result.Text() != wanted {
			test.Fatalf("wrong metadata selection: %q; want %q", result.Text(), wanted)
		}
	}
}

func TestListingRejectsUnknownOrRepeatedFields(test *testing.T) {
	for _, fields := range [][]string{{"unknown"}, {"size", "size"}} {
		result, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), test.TempDir()), fileCall(test, map[string]any{"path": ".", "actions": []any{map[string]any{"op": "list", "fields": fields}}}))
		if operationError == nil || !strings.Contains(result.Error, "listing field") {
			test.Fatalf("invalid fields accepted: %v", fields)
		}
	}
}
