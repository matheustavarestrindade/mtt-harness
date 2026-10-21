package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestInitialSchemaCreatesTablesBeforeTheirIndexes(testingHandle *testing.T) {
	databaseURL := os.Getenv("MTT_TEST_DATABASE_URL")
	if databaseURL == "" {
		testingHandle.Skip("MTT_TEST_DATABASE_URL is not set")
	}
	operationContext := context.Background()
	administration, operationError := pgxpool.New(operationContext, databaseURL)
	testutil.RequireNoError(testingHandle, operationError)
	defer administration.Close()
	schemaName := fmt.Sprintf("fresh_harness_%d", time.Now().UnixNano())
	identifier := pgx.Identifier{schemaName}.Sanitize()
	_, operationError = administration.Exec(operationContext, "CREATE SCHEMA "+identifier)
	testutil.RequireNoError(testingHandle, operationError)
	defer func() {
		_, cleanupError := administration.Exec(operationContext, "DROP SCHEMA "+identifier+" CASCADE")
		testutil.RequireNoError(testingHandle, cleanupError)
	}()
	configuration, operationError := pgxpool.ParseConfig(databaseURL)
	testutil.RequireNoError(testingHandle, operationError)
	configuration.ConnConfig.RuntimeParams["search_path"] = schemaName
	fresh, operationError := pgxpool.NewWithConfig(operationContext, configuration)
	testutil.RequireNoError(testingHandle, operationError)
	defer fresh.Close()
	testutil.RequireNoError(testingHandle, initializeSchema(operationContext, fresh))
	testutil.RequireNoError(testingHandle, initializeSchema(operationContext, fresh))
	var present bool
	operationError = fresh.QueryRow(operationContext, "SELECT to_regclass('messages_session_user') IS NOT NULL").Scan(&present)
	testutil.RequireNoError(testingHandle, operationError)
	if !present {
		testingHandle.Fatal("fresh schema is missing the user-message index")
	}
}
