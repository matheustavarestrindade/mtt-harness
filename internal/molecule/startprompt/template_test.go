package startprompt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestRenderingIsSinglePassAndPreservesLiteralJSON(test *testing.T) {
	template, operationError := Parse(`{{tool_list}} {workspace} {workspace} {tool_list} {"nested":{"value":1}}`)
	testutil.RequireNoError(test, operationError)
	references := []atom.ToolReference{{Name: "second", Categories: []string{"z", "a"}}, {Name: "first", Categories: []string{"files"}}}
	originalCategories := append([]string(nil), references[0].Categories...)
	listCalls := 0
	rendered, operationError := template.Render(context.Background(), Values{
		Workspace: "/workspace/{session_id}",
		ToolList: func() []atom.ToolReference {
			listCalls++
			return references
		},
	})
	testutil.RequireNoError(test, operationError)
	expected := `{tool_list} /workspace/{session_id} /workspace/{session_id} - first [files]
- second [a, z] {"nested":{"value":1}}`
	if rendered != expected || listCalls != 1 {
		test.Fatalf("unexpected template rendering: %q (registry calls %d)", rendered, listCalls)
	}
	if references[0].Name != "second" || !reflect.DeepEqual(references[0].Categories, originalCategories) {
		test.Fatal("formatting modified registry metadata")
	}
}

func TestToolInfoKeepsFullContractAndDoesNotReexpandMetadata(test *testing.T) {
	template, operationError := Parse("{mcp__server__example_info}")
	testutil.RequireNoError(test, operationError)
	schema := json.RawMessage(`{"type":"object","properties":{"timeout":{"type":"integer","description":"Timeout in seconds; zero waits indefinitely.","default":0}}}`)
	rendered, operationError := template.Render(context.Background(), Values{ToolInfo: func(name string) (atom.ToolSpec, error) {
		if name != "mcp__server__example" {
			test.Fatalf("incorrect tool name: %q", name)
		}
		return atom.ToolSpec{Name: name, Description: "Literal {workspace} and {missing_info}", Categories: []string{"mcp"}, InputSchema: atom.Schema{JSON: schema}}, nil
	}})
	testutil.RequireNoError(test, operationError)
	var definition struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		InputSchema json.RawMessage `json:"input_schema"`
	}
	testutil.RequireNoError(test, json.Unmarshal([]byte(rendered), &definition))
	var expectedSchema, renderedSchema any
	testutil.RequireNoError(test, json.Unmarshal(schema, &expectedSchema))
	testutil.RequireNoError(test, json.Unmarshal(definition.InputSchema, &renderedSchema))
	if definition.Name != "mcp__server__example" || definition.Description != "Literal {workspace} and {missing_info}" || !reflect.DeepEqual(renderedSchema, expectedSchema) {
		test.Fatalf("tool contract changed: %s", rendered)
	}
}

func TestRepeatedToolVariableUsesOneLookupPerRequest(test *testing.T) {
	template, operationError := Parse("{example_info}\n{example_info}")
	testutil.RequireNoError(test, operationError)
	lookupCount := 0
	values := Values{ToolInfo: func(name string) (atom.ToolSpec, error) {
		lookupCount++
		return atom.ToolSpec{Name: name, Description: fmt.Sprint(lookupCount)}, nil
	}}
	first, operationError := template.Render(context.Background(), values)
	testutil.RequireNoError(test, operationError)
	second, operationError := template.Render(context.Background(), values)
	testutil.RequireNoError(test, operationError)
	if lookupCount != 2 || first == second {
		test.Fatal("tool information was not cached per request or was cached across requests")
	}
}

func TestInvalidTemplatesAndUnavailableToolsReturnErrors(test *testing.T) {
	for _, source := range []string{"{tool_lsit}", "{unknown}", "{_info}"} {
		if _, operationError := Parse(source); operationError == nil {
			test.Fatalf("unknown placeholder succeeded: %s", source)
		}
	}
	template, operationError := Parse("{missing_info}")
	testutil.RequireNoError(test, operationError)
	missingTool := errors.New("tool was removed")
	_, operationError = template.Render(context.Background(), Values{ToolInfo: func(string) (atom.ToolSpec, error) {
		return atom.ToolSpec{}, missingTool
	}})
	if !errors.Is(operationError, missingTool) || !strings.Contains(operationError.Error(), "missing_info") {
		test.Fatalf("missing tool error lost context: %v", operationError)
	}
	operationContext, cancel := context.WithCancel(context.Background())
	cancel()
	if _, operationError := template.Render(operationContext, Values{}); !errors.Is(operationError, context.Canceled) {
		test.Fatalf("cancellation lost: %v", operationError)
	}
}

func TestLoadKeepsTemplateUntilRestartAndReportsFileErrors(test *testing.T) {
	path := filepath.Join(test.TempDir(), "start_prompt.md")
	if _, operationError := Load(path); !errors.Is(operationError, os.ErrNotExist) {
		test.Fatalf("missing prompt file succeeded: %v", operationError)
	}
	testutil.RequireNoError(test, os.WriteFile(path, []byte("original"), 0600))
	template, operationError := Load(path)
	testutil.RequireNoError(test, operationError)
	testutil.RequireNoError(test, os.WriteFile(path, []byte("changed"), 0600))
	rendered, operationError := template.Render(context.Background(), Values{})
	testutil.RequireNoError(test, operationError)
	if rendered != "original" {
		test.Fatal("loaded template changed without a reload")
	}
	testutil.RequireNoError(test, os.WriteFile(path, nil, 0600))
	template, operationError = Load(path)
	testutil.RequireNoError(test, operationError)
	rendered, operationError = template.Render(context.Background(), Values{})
	testutil.RequireNoError(test, operationError)
	if rendered != "" {
		test.Fatal("empty template supplied hidden instructions")
	}
}

func TestConcurrentSessionsShareOnlyTemplateText(test *testing.T) {
	template, operationError := Parse("{session_id} {workspace} {session_id}")
	testutil.RequireNoError(test, operationError)
	for index := range 16 {
		test.Run(fmt.Sprint(index), func(test *testing.T) {
			test.Parallel()
			sessionID := fmt.Sprintf("session-%d", index)
			workspace := fmt.Sprintf("/workspace/%d", index)
			values := Values{Session: atom.Session{ID: atom.SessionID(sessionID)}, Workspace: workspace}
			for range 20 {
				rendered, operationError := template.Render(context.Background(), values)
				testutil.RequireNoError(test, operationError)
				if rendered != sessionID+" "+workspace+" "+sessionID {
					test.Fatalf("values leaked across sessions: %q", rendered)
				}
			}
		})
	}
}
