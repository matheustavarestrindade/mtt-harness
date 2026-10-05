package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/eventbus"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/process"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/processes"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
	"github.com/matheustavarestrindade/mtt-harness/plugins/pathtools"
)

func TestFilesystemAndShellUseInstanceWorkspace(test *testing.T) {
	serverDirectory, workspace := test.TempDir(), test.TempDir()
	test.Chdir(serverDirectory)
	testutil.RequireNoError(test, os.WriteFile(filepath.Join(serverDirectory, "same.txt"), []byte("server"), 0600))
	testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, "same.txt"), []byte("instance"), 0600))
	session := atom.Session{ID: "session", InstanceID: "instance"}
	operationContext := harness.WithWorkspace(harness.WithSession(context.Background(), session), workspace)
	result, operationError := (Read{}).Run(operationContext, atom.ToolCall{Input: []byte(`{"path":"same.txt"}`)})
	testutil.RequireNoError(test, operationError)
	if result.Text() != "instance" {
		test.Fatalf("wrong file: %q", result.Text())
	}
	_, operationError = (Write{}).Run(operationContext, atom.ToolCall{Input: []byte(`{"path":"created.txt","content":"inside"}`)})
	testutil.RequireNoError(test, operationError)
	if _, operationError := os.Stat(filepath.Join(serverDirectory, "created.txt")); !os.IsNotExist(operationError) {
		test.Fatal("write escaped into server directory")
	}
	runtime := harness.New()
	database := memory.New()
	manager := processes.New(process.New(1), database, runtime, eventbus.New(runtime), nil)
	defer func() {
		shutdownContext, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		testutil.RequireNoError(test, manager.Close(shutdownContext))
	}()
	result, operationError = NewBash(manager).Run(operationContext, atom.ToolCall{Input: []byte(`{"command":"pwd"}`)})
	testutil.RequireNoError(test, operationError)
	if strings.TrimSpace(result.Text()) != workspace {
		test.Fatalf("shell directory: %q", result.Text())
	}
}

func TestPathGuardResolvesMissingFileThroughSymlink(test *testing.T) {
	workspace, outside := test.TempDir(), test.TempDir()
	testutil.RequireNoError(test, os.Symlink(outside, filepath.Join(workspace, "link")))
	runtime := harness.New()
	guard := &pathtools.PathGuard{}
	testutil.RequireNoError(test, guard.Setup(runtime))
	operationContext := harness.WithWorkspace(context.Background(), workspace)
	verdict, operationError := harness.Check(operationContext, runtime, atom.StageToolInput, atom.ToolCall{Name: "write", Input: []byte(`{"path":"link/new.txt"}`)})
	testutil.RequireNoError(test, operationError)
	if verdict.Kind != atom.VerdictAsk || verdict.Target != filepath.Join(outside, "new.txt") {
		test.Fatalf("symlink check: %+v", verdict)
	}
}

func TestDeletionPathGuardUsesTheEntryWithoutFollowingItsTarget(test *testing.T) {
	workspace, outside := test.TempDir(), test.TempDir()
	target := filepath.Join(outside, "target.txt")
	testutil.RequireNoError(test, os.WriteFile(target, []byte("outside content"), 0o600))
	testutil.RequireNoError(test, os.Symlink(target, filepath.Join(workspace, "link")))
	testutil.RequireNoError(test, os.Symlink(outside, filepath.Join(workspace, "parent-link")))
	harnessRuntime := harness.New()
	testutil.RequireNoError(test, (&pathtools.PathGuard{}).Setup(harnessRuntime))
	operationContext := harness.WithWorkspace(context.Background(), workspace)
	for _, scenario := range []struct {
		input  string
		kind   atom.VerdictKind
		target string
	}{
		{`{"path":"link","actions":[{"op":"delete"}],"return":{"type":"summary"}}`, atom.VerdictAllow, ""},
		{`{"path":"link","actions":[{"op":"read"}]}`, atom.VerdictAsk, target},
		{`{"path":"parent-link/target.txt","actions":[{"op":"delete"}],"return":{"type":"summary"}}`, atom.VerdictAsk, target},
	} {
		verdict, operationError := harness.Check(operationContext, harnessRuntime, atom.StageToolInput, atom.ToolCall{Name: "file_actions", Input: []byte(scenario.input)})
		testutil.RequireNoError(test, operationError)
		if verdict.Kind != scenario.kind || verdict.Target != scenario.target {
			test.Fatalf("path policy changed the deletion target: %+v", verdict)
		}
	}
	result, operationError := NewFileActions(false).Run(operationContext, atom.ToolCall{Name: "file_actions", Input: []byte(`{"path":"link","actions":[{"op":"read"},{"op":"delete"}],"return":{"type":"summary"}}`)})
	if operationError == nil || strings.Contains(result.Text(), "outside content") {
		test.Fatal("mixed deletion chain followed an unchecked final symlink")
	}
	content, operationError := os.ReadFile(target)
	testutil.RequireNoError(test, operationError)
	if string(content) != "outside content" {
		test.Fatal("deletion changed the symlink target")
	}
	verdict, operationError := harness.Check(operationContext, harnessRuntime, atom.StageToolInput, atom.ToolCall{Name: "mcp__custom", Input: []byte(`{"path":"file.txt","actions":"server-owned format"}`)})
	testutil.RequireNoError(test, operationError)
	if verdict.Kind != atom.VerdictAllow {
		test.Fatal("file action parsing changed an external tool's input")
	}
}
