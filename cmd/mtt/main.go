package main

import (
	"errors"
	"flag"
	"log"
	"os"
)

func main() {
	if operationError := runCommandLine(os.Args[1:]); operationError != nil {
		log.Printf("mtt: %v", operationError)
		os.Exit(1)
	}
}

func runCommandLine(arguments []string) (operationError error) {
	defer captureStartupFailure(&operationError)
	configuration, configurationError := parseArguments(arguments)
	if errors.Is(configurationError, flag.ErrHelp) {
		return nil
	}
	requireStartupSuccess(configurationError, "load startup configuration")
	return runApplication(configuration)
}
