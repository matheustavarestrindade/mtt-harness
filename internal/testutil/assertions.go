// Package testutil provides shared assertions for harness tests.
package testutil

import "testing"

// RequireNoError stops the test at its caller when an operation fails.
func RequireNoError(test testing.TB, operationError error) {
	test.Helper()
	if operationError == nil {
		return
	}
	test.Fatal(operationError)
}
