package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestStartupFailurePreservesCauseAndRunsCleanup(test *testing.T) {
	cause := errors.New("database unavailable")
	cleanedUp := false
	start := func() (operationError error) {
		defer captureStartupFailure(&operationError)
		defer func() {
			cleanedUp = true
		}()
		requireStartupSuccess(cause, "open database")
		test.Fatal("startup continued after a required operation failed")
		return nil
	}
	operationError := start()
	if !errors.Is(operationError, cause) || !strings.Contains(operationError.Error(), "open database") {
		test.Fatalf("startup error lost its operation or cause: %v", operationError)
	}
	if !cleanedUp {
		test.Fatal("startup did not release acquired resources")
	}
}

func TestStartupRecoveryDoesNotHideProgrammingPanics(test *testing.T) {
	programmingError := errors.New("unexpected programming panic")
	defer func() {
		if recovered := recover(); recovered != programmingError {
			test.Fatalf("unexpected panic was swallowed or replaced: %v", recovered)
		}
	}()
	var operationError error
	defer captureStartupFailure(&operationError)
	panic(programmingError)
}

func TestRunReturnsInvalidConfigurationError(test *testing.T) {
	configurationPath := filepath.Join(test.TempDir(), "mtt.json")
	testutil.RequireNoError(test, os.WriteFile(configurationPath, []byte("invalid JSON"), 0600))
	if operationError := runCommandLine([]string{"--config", configurationPath}); operationError == nil {
		test.Fatal("invalid bootstrap configuration did not fail startup")
	}
}
