package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"

	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

func resolveToken(operationContext context.Context, database store.Store, configuredToken string) string {
	if configuredToken != "" {
		requireStartupSuccess(database.Settings().Save(operationContext, "", "api_token", configuredToken), "set bootstrap API token")
		return configuredToken
	}
	if token, operationError := database.Settings().Get(operationContext, "", "api_token"); operationError == nil && token != "" {
		return token
	}
	token := newToken()
	requireStartupSuccess(database.Settings().Save(operationContext, "", "api_token", token), "save initial API token")
	fmt.Printf("mtt API token: %s\n", token)
	log.Printf("mtt: the first start makes the API token. Do not lose it.")
	return token
}

func newToken() string {
	var tokenBytes [32]byte
	_, operationError := rand.Read(tokenBytes[:])
	requireStartupSuccess(operationError, "generate API token")
	return hex.EncodeToString(tokenBytes[:])
}
