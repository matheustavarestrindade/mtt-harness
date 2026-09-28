package main

import (
	"context"
	"log"
	"strconv"

	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	memorystore "github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/postgres"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
)

func openStore(operationContext context.Context, databaseURL string) store.Store {
	if databaseURL == "" {
		return memorystore.New()
	}
	postgresStore, operationError := postgres.Open(operationContext, databaseURL)
	requireStartupSuccess(operationError, "open configured Postgres database")
	log.Printf("mtt: the Postgres store is in use")
	return postgresStore
}

func restoreInstances(operationContext context.Context, database store.Store, instanceManager *instances.Manager) {
	instanceSpecs, operationError := database.Instances().All(operationContext)
	requireStartupSuccess(operationError, "read saved instances")
	for _, instanceSpec := range instanceSpecs {
		if instanceSpec.Stopped {
			continue
		}
		instance, operationError := instanceManager.Start(operationContext, instanceSpec)
		if operationError != nil {
			instanceSpec.Stopped = true
			requireStartupSuccess(database.Instances().Save(operationContext, instanceSpec), "preserve unavailable instance")
			log.Printf("mtt: instance %s remains stopped: %v", instanceSpec.ID, operationError)
			continue
		}
		requireStartupSuccess(database.Instances().Save(operationContext, instance.Spec()), "save migrated instance settings")
	}
}

func globalProcessLimit(operationContext context.Context, database store.Store) int {
	value, operationError := database.Settings().Get(operationContext, "", "process_limit")
	if operationError != nil || value == "" {
		return 8
	}
	limit, operationError := strconv.Atoi(value)
	if operationError != nil || limit <= 0 {
		return 8
	}
	return limit
}
