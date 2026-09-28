// Package operation adds context to failures while preserving their cause.
package operation

import "fmt"

// WrapError adds the operation name to a failure. A successful operation stays nil.
func WrapError(operationError error, description string) error {
	if operationError == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", description, operationError)
}
