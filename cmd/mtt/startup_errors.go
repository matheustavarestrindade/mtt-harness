package main

import "github.com/matheustavarestrindade/mtt-harness/internal/operation"

// startupFailure is used only by the startup helpers on the main goroutine.
// Defers release acquired resources before run reports the failure to main.
type startupFailure struct {
	operationError error
}

func requireStartupSuccess(operationError error, description string) {
	if operationError == nil {
		return
	}
	panic(startupFailure{operation.WrapError(operationError, description)})
}

// Programming panics are not startup errors and must retain their stack traces.
func captureStartupFailure(operationError *error) {
	recovered := recover()
	if recovered == nil {
		return
	}
	failure, expected := recovered.(startupFailure)
	if !expected {
		panic(recovered)
	}
	*operationError = failure.operationError
}
