package workspaceread

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestRootedRetrievalExcludesSecretsLinksAndBinaryData(test *testing.T) {
	root := test.TempDir()
	outside := test.TempDir()
	for path, text := range map[string]string{"AGENTS.md": "Use PostgreSQL for the database.\n", "src/database.go": "package database\n// PostgreSQL migrations retain old data.\n", ".env": "PostgreSQL_PASSWORD=secret", "credentials": "PostgreSQL token", "node_modules/private.js": "PostgreSQL unwanted dependency", "binary.txt": "PostgreSQL\x00secret"} {
		filename := filepath.Join(root, path)
		testutil.RequireNoError(test, os.MkdirAll(filepath.Dir(filename), 0755))
		testutil.RequireNoError(test, os.WriteFile(filename, []byte(text), 0600))
	}
	testutil.RequireNoError(test, os.WriteFile(filepath.Join(outside, "escape.go"), []byte("PostgreSQL external secret"), 0600))
	testutil.RequireNoError(test, os.Symlink(outside, filepath.Join(root, "linked-directory")))
	testutil.RequireNoError(test, os.Symlink(filepath.Join(root, ".env"), filepath.Join(root, "linked.go")))
	result, operationError := Search(context.Background(), root, harness.WorkspaceFileQuery{Query: "PostgreSQL database migrations", Limit: 10, MaxBytes: 4096})
	testutil.RequireNoError(test, operationError)
	if len(result.References) != 2 {
		test.Fatalf("unexpected files: %+v", result)
	}
	for _, reference := range result.References {
		if reference.Path != "AGENTS.md" && reference.Path != "src/database.go" {
			test.Fatalf("unexpected reference: %+v", reference)
		}
		if reference.StartLine < 1 || reference.EndLine < reference.StartLine || reference.Version == "" {
			test.Fatalf("missing provenance: %+v", reference)
		}
	}
	valid, operationError := Verify(context.Background(), root, result.References)
	testutil.RequireNoError(test, operationError)
	if !valid {
		test.Fatal("fresh source rejected")
	}
	testutil.RequireNoError(test, os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("Changed decision"), 0600))
	valid, operationError = Verify(context.Background(), root, result.References)
	testutil.RequireNoError(test, operationError)
	if valid {
		test.Fatal("changed source accepted")
	}
	valid, operationError = Verify(context.Background(), root, []harness.WorkspaceFileReference{{Path: "../escape.go"}})
	testutil.RequireNoError(test, operationError)
	if valid {
		test.Fatal("path traversal accepted")
	}
}

func TestReadBoundsAndCancellation(test *testing.T) {
	root := test.TempDir()
	testutil.RequireNoError(test, os.WriteFile(filepath.Join(root, "large.go"), []byte(strings.Repeat("database ", 20000)), 0600))
	result, operationError := Search(context.Background(), root, harness.WorkspaceFileQuery{Query: "database", Limit: 1, MaxBytes: 256})
	testutil.RequireNoError(test, operationError)
	if len(result.References) != 0 || result.BytesExamined > maximumReadBytes {
		test.Fatalf("bounds failed: %+v", result)
	}
	operationContext, cancel := context.WithCancel(context.Background())
	cancel()
	if _, operationError := Search(operationContext, root, harness.WorkspaceFileQuery{Query: "database"}); operationError == nil {
		test.Fatal("cancelled search succeeded")
	}
}
