//go:build gemma && cgo

package contextplugin

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/embedding/embeddinggemma"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestRealGemmaSavesVerbatimAndQueriesWorkspaceVectors(test *testing.T) {
	directory, library := os.Getenv("MTT_TEST_CONTEXT_MODEL_DIRECTORY"), os.Getenv("MTT_TEST_CONTEXT_RUNTIME_LIBRARY")
	if directory == "" || library == "" {
		test.Skip("real Gemma assets are not configured")
	}
	encoder, operationError := embeddinggemma.New(embeddinggemma.Configuration{ModelDirectory: directory, ModelFile: "onnx/model_quantized.onnx", RuntimeLibrary: library, ChunkTokens: 1024, Threads: 2})
	testutil.RequireNoError(test, operationError)
	test.Cleanup(func() { testutil.RequireNoError(test, encoder.Close()) })
	fixture := newPluginFixtureWithEmbeddings(test, encoder)
	notes := []string{
		"  My name is Matheus.\nPlease use this name.  ",
		"PostgreSQL stores durable application records and provides database transactions.",
		"The user prefers pizza and cold soft drinks for lunch.",
	}
	for index, note := range notes {
		receipt, operationError := fixture.plugin.saveMemory(context.Background(), fixture.session, "gemma-note-"+strconv.Itoa(index), rememberInput{Text: note, Categories: []string{"facts"}})
		testutil.RequireNoError(test, operationError)
		encoded, operationError := json.Marshal(receipt)
		testutil.RequireNoError(test, operationError)
		if string(encoded) != `{"state":"saved"}` {
			test.Fatalf("unexpected receipt: %+v", receipt)
		}
	}
	for _, testCase := range []struct {
		query    string
		expected string
	}{
		{"What is the user called?", notes[0]},
		{"Which storage engine provides transactional persistence?", notes[1]},
		{"Which foods and drinks does the user enjoy?", notes[2]},
	} {
		reply, operationError := fixture.plugin.searchMemory(context.Background(), fixture.session, memorySearchInput{Query: testCase.query, Compression: "low", Limit: 1})
		testutil.RequireNoError(test, operationError)
		if len(reply.Matches) != 1 || reply.Matches[0].Data != testCase.expected {
			test.Fatalf("wrong semantic result for %q: %+v", testCase.query, reply)
		}
	}
	other := fixture.session
	other.InstanceID = "different-workspace"
	reply, operationError := fixture.plugin.searchMemory(context.Background(), other, memorySearchInput{Query: "Matheus", Compression: "low"})
	testutil.RequireNoError(test, operationError)
	if len(reply.Matches) != 0 {
		test.Fatal("Gemma retrieval crossed workspace ownership")
	}
	fixture.services.mutex.Lock()
	defer fixture.services.mutex.Unlock()
	if len(fixture.services.calls) != 0 {
		test.Fatal("direct Gemma memory saves invoked a language model")
	}
}
