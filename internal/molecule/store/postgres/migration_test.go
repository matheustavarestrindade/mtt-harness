package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestUnchangedMigrationDoesNotLockActiveWriterTables(test *testing.T) {
	databaseURL := os.Getenv("MTT_TEST_DATABASE_URL")
	if databaseURL == "" {
		test.Skip("Postgres test database is not configured")
	}
	database, operationError := Open(context.Background(), databaseURL)
	testutil.RequireNoError(test, operationError)
	defer database.Close()
	writer, operationError := database.pool.Begin(context.Background())
	testutil.RequireNoError(test, operationError)
	defer writer.Rollback(context.Background())
	// Guarded writers hold table/row locks before reading session tombstones.
	// An unchanged startup must not request ACCESS EXCLUSIVE on these tables.
	_, operationError = writer.Exec(context.Background(), `LOCK TABLE messages IN ROW EXCLUSIVE MODE; LOCK TABLE sessions IN ROW SHARE MODE`)
	testutil.RequireNoError(test, operationError)
	operationContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	reopened, operationError := Open(operationContext, databaseURL)
	testutil.RequireNoError(test, operationError)
	defer reopened.Close()
}
