package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/api"
	"github.com/matheustavarestrindade/mtt-harness/internal/config"
	"github.com/matheustavarestrindade/mtt-harness/internal/mcp"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/eventbus"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/permission"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/process"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	memorystore "github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/postgres"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/plugins"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/processes"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/registry"
	"github.com/matheustavarestrindade/mtt-harness/internal/tools"
	"github.com/matheustavarestrindade/mtt-harness/plugins/pathtools"
)

func main() {
	configPath := flag.String("config", "mtt.json", "the bootstrap config file")
	port := flag.Int("port", 0, "the API port")
	databaseURL := flag.String("database-url", "", "the Postgres URL")
	providersFile := flag.String("providers-file", "", "the providers file")
	mcpFile := flag.String("mcp-file", "", "the MCP server file")
	testProvider := flag.Bool("test-provider", false, "use the test provider")
	flag.Parse()

	bootstrap, err := config.LoadOrDefault(*configPath)
	if err != nil {
		log.Fatalf("mtt: %v", err)
	}
	if *port != 0 {
		bootstrap.Port = *port
	}
	if *databaseURL != "" {
		bootstrap.DatabaseURL = *databaseURL
	}
	if *providersFile != "" {
		bootstrap.ProvidersFile = *providersFile
	}
	if *mcpFile != "" {
		bootstrap.MCPFile = *mcpFile
	}
	if *testProvider {
		bootstrap.TestProvider = true
	}

	ctx := context.Background()
	database := openStore(ctx, bootstrap.DatabaseURL)
	defer database.Close()
	bus := eventbus.New()
	bus.SetRecorder(func(ctx context.Context, event atom.Event) {
		_ = database.Events().Append(ctx, event)
	})

	token := resolveToken(ctx, database, bootstrap.APIToken)

	h := harness.New()
	reg := registry.New()
	engine := permission.NewEngine()
	broker := permission.NewBroker()
	supervisor := process.New(globalProcessLimit(ctx, database))
	models := gateway.New()
	instanceManager := instances.New(func(instanceID string) instances.SessionManager {
		return memory.New(instanceID, database.Sessions())
	}, database.Settings())
	processManager := processes.New(supervisor, database, h, bus, instanceManager)
	runner := loop.New(h, loop.Config{
		Gateway:   models,
		Registry:  reg,
		Store:     database,
		Bus:       bus,
		Broker:    broker,
		Engine:    engine,
		Instances: instanceManager,
	})

	for _, tool := range []harness.Tool{
		tools.NewBash(processManager),
		tools.Read{},
		tools.Write{},
		tools.NewSearch(reg),
		tools.NewProcessOutput(processManager),
		tools.NewProcessKill(processManager),
		tools.Finish{},
		&tools.Agent{RunTask: runner.RunAgentTask},
	} {
		if err := reg.Add(tool); err != nil {
			log.Fatalf("mtt: %v", err)
		}
		h.Tool(tool)
	}

	host := plugins.New(h)
	if err := host.Attach(&pathtools.PathGuard{
		WorkspaceOf: func(instanceID string) string {
			instance, ok := instanceManager.Get(instanceID)
			if !ok {
				return ""
			}
			return instance.Workspace()
		},
	}); err != nil {
		log.Fatalf("mtt: %v", err)
	}

	if bootstrap.TestProvider {
		test := provider.NewTest("test", provider.Text("the test provider is active"))
		if err := models.Add(test); err != nil {
			log.Fatalf("mtt: %v", err)
		}
		h.Provider(test)
		log.Printf("mtt: the test provider is in use")
	} else {
		loadProviders(ctx, bootstrap.ProvidersFile, models, h, database)
	}
	loadMCP(ctx, bootstrap.MCPFile, reg)

	server := &http.Server{
		Addr: ":" + strconv.Itoa(bootstrap.Port),
		Handler: api.New(api.Config{
			Token:     token,
			Store:     database,
			Instances: instanceManager,
			Bus:       bus,
			Broker:    broker,
			Loop:      runner,
			Processes: processManager,
			Gateway:   models,
		}).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	signalContext, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-signalContext.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()

	log.Printf("mtt: the API is on port %d", bootstrap.Port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("mtt: %v", err)
	}
}

func resolveToken(ctx context.Context, database store.Store, fromFile string) string {
	if fromFile != "" {
		return fromFile
	}
	if value, err := database.Settings().Get(ctx, "", "api_token"); err == nil && value != "" {
		return value
	}
	token := newToken()
	if err := database.Settings().Save(ctx, "", "api_token", token); err != nil {
		log.Fatalf("mtt: %v", err)
	}
	fmt.Printf("mtt API token: %s\n", token)
	log.Printf("mtt: the first start makes the API token. Do not lose it.")
	return token
}

func globalProcessLimit(ctx context.Context, database store.Store) int {
	if value, err := database.Settings().Get(ctx, "", "process_limit"); err == nil && value != "" {
		if number, err := strconv.Atoi(value); err == nil && number > 0 {
			return number
		}
	}
	return 8
}

func loadProviders(ctx context.Context, path string, models *gateway.Gateway, h *harness.Harness, database store.Store) {
	configs, err := provider.LoadFile(path)
	switch {
	case err == nil:
		for _, config := range configs {
			standard := provider.New(config.Spec)
			standard.SetPrices(config.Prices)
			standard.SetKeyResolver(func(ctx context.Context, instanceID string, name string) (string, error) {
				return database.Secrets().ResolveKey(ctx, instanceID, name)
			})
			if cached, err := database.Providers().Models(ctx, config.Spec.Name); err == nil && len(cached) > 0 {
				standard.SetModels(cached)
			} else if len(config.Models) > 0 {
				standard.SetModels(config.Models)
			}
			if err := models.Add(standard); err != nil {
				log.Fatalf("mtt: %v", err)
			}
			h.Provider(standard)
			if err := database.Providers().Save(ctx, config.Spec); err != nil {
				log.Fatalf("mtt: %v", err)
			}
			refresh(ctx, models, database, config.Spec)
			log.Printf("mtt: the provider %s is in use", config.Spec.Name)
		}
	case errors.Is(err, os.ErrNotExist):
		log.Printf("mtt: the provider file %s is not there", path)
	default:
		log.Fatalf("mtt: %v", err)
	}
}

var mcpTools sync.Map

func loadMCP(ctx context.Context, path string, reg *registry.Registry) {
	servers, err := mcp.LoadFile(path)
	switch {
	case err == nil:
		for _, server := range servers {
			if !server.Enabled {
				continue
			}
			client, err := mcp.Start(ctx, server, 30*time.Second)
			if err != nil {
				log.Printf("mtt: the MCP server %s: %v", server.Name, err)
				continue
			}
			defer client.Close()
			if err := registerMCPTools(ctx, client, server.Name, reg); err != nil {
				log.Printf("mtt: the MCP server %s: %v", server.Name, err)
				continue
			}
			current := client
			name := server.Name
			client.OnToolsChanged(func(ctx context.Context) {
				if err := registerMCPTools(ctx, current, name, reg); err != nil {
					log.Printf("mtt: the MCP server %s: %v", name, err)
				}
			})
			log.Printf("mtt: the MCP server %s is connected", server.Name)
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		log.Fatalf("mtt: %v", err)
	}
}

func registerMCPTools(ctx context.Context, client *mcp.Client, server string, reg *registry.Registry) error {
	tools, err := client.ListTools(ctx)
	if err != nil {
		return err
	}
	if previous, ok := mcpTools.Load(server); ok {
		for _, name := range previous.([]string) {
			reg.Remove(name)
		}
	}
	var names []string
	for _, spec := range tools {
		tool := &mcp.Tool{Client: client, Server: server, Spec: spec}
		reg.Upsert(tool)
		names = append(names, tool.Name())
	}
	mcpTools.Store(server, names)
	return nil
}

func refresh(ctx context.Context, models *gateway.Gateway, database store.Store, spec atom.ProviderSpec) {
	if spec.ModelListURL == "" {
		return
	}
	if refreshed, err := models.Refresh(ctx, spec.Name); err == nil {
		_ = database.Providers().SaveModels(ctx, spec.Name, refreshed)
		log.Printf("mtt: the provider %s gives %d models", spec.Name, len(refreshed))
	} else {
		log.Printf("mtt: the provider refresh is not complete: %v", err)
	}
	if spec.Interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(spec.Interval)
		defer ticker.Stop()
		for range ticker.C {
			if refreshed, err := models.Refresh(ctx, spec.Name); err == nil {
				_ = database.Providers().SaveModels(ctx, spec.Name, refreshed)
			}
		}
	}()
}

func openStore(ctx context.Context, dsn string) store.Store {
	if dsn != "" {
		if postgresStore, err := postgres.Open(ctx, dsn); err == nil {
			log.Printf("mtt: the Postgres store is in use")
			return postgresStore
		} else {
			log.Printf("mtt: Postgres is not available (%v); the memory store is in use", err)
		}
	}
	return memorystore.New()
}

func newToken() string {
	var data [32]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "change-me"
	}
	return hex.EncodeToString(data[:])
}
