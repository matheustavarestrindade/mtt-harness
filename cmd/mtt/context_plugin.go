package main

import (
	"context"
	"errors"
	"io"
	"log"

	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/embedding"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/plugins"
	contextplugin "github.com/matheustavarestrindade/mtt-harness/plugins/context"
)

func attachContextPlugin(operationContext context.Context, pluginHost *plugins.Host, database store.Store, instanceManager *instances.Manager, modelGateway *gateway.Gateway, databaseURL, providersFile string) (io.Closer, error) {
	configuration, operationError := embedding.LoadContextConfiguration(providersFile)
	if operationError != nil {
		return nil, operationError
	}
	embeddings, identity, embeddingCloser, embeddingError := newContextEmbeddings(operationContext, configuration)
	if embeddingError != nil {
		log.Printf("mtt: context embeddings unavailable: %v", embeddingError)
	}
	if embeddings != nil {
		log.Printf("mtt: context embeddings configured backend=%s identity=%s chunk_tokens=%d threads=%d", configuration.Backend, identity, configuration.ChunkTokens, configuration.Threads)
	}
	services := &plugins.Services{Gateway: modelGateway, Instances: instanceManager, Store: database}
	plugin, operationError := contextplugin.New(operationContext, contextplugin.Options{DatabaseURL: databaseURL, EmbeddingModel: identity, Services: harness.PluginServices{Settings: database.Settings(), Conversations: services, Workspaces: services, Models: services, Embeddings: embeddings, Usage: database.Usage()}})
	if operationError != nil {
		if embeddingCloser != nil {
			operationError = errors.Join(operationError, embeddingCloser.Close())
		}
		return nil, operationError
	}
	if operationError := pluginHost.Attach(plugin); operationError != nil {
		operationError = errors.Join(operationError, plugin.Close(context.Background()))
		if embeddingCloser != nil {
			operationError = errors.Join(operationError, embeddingCloser.Close())
		}
		return nil, operationError
	}
	state, operationError := plugin.ReadState(operationContext, "")
	if operationError != nil {
		return embeddingCloser, operationError
	}
	if state.Error != "" {
		log.Printf("mtt: context plugin unavailable: %s", state.Error)
	}
	return embeddingCloser, nil
}
