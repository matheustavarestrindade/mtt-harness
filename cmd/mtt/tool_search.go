package main

import (
	"context"
	"log"

	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/toolsearch"
)

func loadToolSearch(operationContext context.Context, providersFile string) *toolsearch.Selector {
	configuration, operationError := toolsearch.LoadConfiguration(providersFile)
	requireStartupSuccess(operationError, "load tool search configuration")
	selector, operationError := toolsearch.NewSelector(operationContext, configuration.Mode,
		func(operationContext context.Context) (toolsearch.Searcher, error) {
			return newSemanticSearch(operationContext, configuration)
		},
		toolsearch.NewLexical(configuration.LexicalMinimumSimilarity),
		func(operationError error) {
			log.Printf("mtt: tool search selected lexical fallback: %v", operationError)
		},
	)
	requireStartupSuccess(operationError, "initialize tool search")
	log.Printf("mtt: tool search requested %s, selected %s", configuration.Mode, selector.SelectedMode())
	return selector
}
