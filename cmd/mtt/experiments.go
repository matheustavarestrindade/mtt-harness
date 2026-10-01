package main

import (
	"fmt"
	"os"
	"strconv"
)

// This process-level benchmark switch is intentionally separate from persistent
// user settings and bootstrap files. Docker Compose loads .env and forwards the
// value; the Go process reads its environment once during tool registration.
func readLineNumbersExperiment() (bool, error) {
	value := os.Getenv("MTT_READ_LINE_NUMBERS")
	if value == "" {
		return false, nil
	}
	enabled, operationError := strconv.ParseBool(value)
	if operationError != nil {
		return false, fmt.Errorf("MTT_READ_LINE_NUMBERS must be true/false or 1/0: %w", operationError)
	}
	return enabled, nil
}
