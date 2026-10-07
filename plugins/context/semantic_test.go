//go:build semantic

package contextplugin

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/embedding/minilm"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestRealMiniLMQueriesPostgresVectors(test *testing.T) {
	directory := os.Getenv("MTT_TEST_SEARCH_MODEL_DIRECTORY")
	if directory == "" {
		test.Skip("real MiniLM assets are not configured")
	}
	encoder, operationError := minilm.New(directory)
	testutil.RequireNoError(test, operationError)
	test.Cleanup(func() { testutil.RequireNoError(test, encoder.Close()) })
	fixture := newPluginFixtureWithEmbeddings(test, encoder)
	seedMemory(test, fixture, "Database", "PostgreSQL stores durable application records and provides database transactions.", false)
	seedMemory(test, fixture, "Lunch", "The user prefers pizza and cold soft drinks for lunch.", false)
	reply, operationError := fixture.plugin.searchMemory(context.Background(), fixture.session, memorySearchInput{Query: "Which storage engine provides transactional persistence?", Compression: "low", Limit: 1})
	testutil.RequireNoError(test, operationError)
	if len(reply.Matches) != 1 || !strings.Contains(reply.Matches[0].Data, "PostgreSQL") {
		test.Fatalf("real semantic retrieval failed: %+v", reply)
	}
}
