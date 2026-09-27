package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/api"
	"github.com/matheustavarestrindade/mtt-harness/internal/bridge"
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
	loadDotEnv(".env")

	port := env("MTT_PORT", "8080")
	token := env("MTT_API_TOKEN", "")
	workspace := env("MTT_WORKSPACE", ".")
	dsn := os.Getenv("MTT_DATABASE_URL")

	ctx := context.Background()
	database := openStore(ctx, dsn)
	bus := eventbus.New()
	bus.SetRecorder(func(ctx context.Context, event atom.Event) {
		_ = database.Events().Append(ctx, event)
	})

	h := harness.New()
	reg := registry.New()
	engine := permission.NewEngine()
	broker := permission.NewBroker(time.Duration(envInt("MTT_PERMISSION_TIMEOUT_SECONDS", 300)) * time.Second)
	supervisor := process.New(envInt("MTT_PROCESS_LIMIT", 8))
	models := gateway.New()
	instanceManager := instances.New(func(instanceID string) instances.SessionManager {
		return memory.New(instanceID, database.Sessions())
	})
	processManager := processes.New(supervisor, database, h, bus)
	runner := loop.New(h, loop.Config{
		Gateway:   models,
		Registry:  reg,
		Store:     database,
		Bus:       bus,
		Broker:    broker,
		Engine:    engine,
		Processes: processManager,
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
	if err := host.Attach(&pathtools.PathGuard{Workspace: workspace}); err != nil {
		log.Fatalf("mtt: %v", err)
	}

	if os.Getenv("MTT_TEST_PROVIDER") == "1" {
		testProvider := provider.NewTest("test", provider.Text("the test provider is active"))
		if err := models.Add(testProvider); err != nil {
			log.Fatalf("mtt: %v", err)
		}
		h.Provider(testProvider)
		if err := startInstance(ctx, instanceManager, database, "test-model"); err != nil {
			log.Fatalf("mtt: %v", err)
		}
		log.Printf("mtt: the test provider is in use")
	} else if apiURL := os.Getenv("MTT_PROVIDER_API_URL"); apiURL != "" {
		spec := atom.ProviderSpec{
			Name:          env("MTT_PROVIDER_NAME", "default"),
			APIURL:        apiURL,
			ModelListURL:  os.Getenv("MTT_PROVIDER_MODEL_LIST_URL"),
			PriceTableURL: os.Getenv("MTT_PROVIDER_PRICE_TABLE_URL"),
			Secret:        os.Getenv("MTT_PROVIDER_API_KEY"),
			Interval:      time.Duration(envInt("MTT_PROVIDER_REFRESH_HOURS", 24)) * time.Hour,
		}
		standard := provider.New(spec)
		if cached, err := database.Providers().Models(ctx, spec.Name); err == nil && len(cached) > 0 {
			standard.SetModels(cached)
		}
		if err := models.Add(standard); err != nil {
			log.Fatalf("mtt: %v", err)
		}
		h.Provider(standard)
		if err := database.Providers().Save(ctx, spec); err != nil {
			log.Fatalf("mtt: %v", err)
		}
		refresh(ctx, models, database, spec)
		if err := startInstance(ctx, instanceManager, database, defaultModel(models, spec.Name)); err != nil {
			log.Fatalf("mtt: %v", err)
		}
	}

	if command := os.Getenv("MTT_PLUGIN_COMMAND"); command != "" {
		parts := strings.Fields(command)
		external, err := bridge.Start(ctx, bridge.Config{Harness: h, Registry: reg}, parts[0], parts[1:]...)
		if err != nil {
			log.Fatalf("mtt: %v", err)
		}
		defer external.Close()
		bus.On("*", func(ctx context.Context, event atom.Event) {
			external.Notify(event)
		})
		log.Printf("mtt: the bridge is connected to %s", command)
	}

	server := &http.Server{
		Addr: ":" + port,
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

	log.Printf("mtt: the API is on port %s", port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("mtt: %v", err)
	}
}

func startInstance(ctx context.Context, manager *instances.Manager, database store.Store, defaultModel string) error {
	instance, err := manager.Start(ctx, atom.InstanceSpec{
		Workspace:       env("MTT_WORKSPACE", "."),
		DefaultModel:    defaultModel,
		AgentDepthLimit: envInt("MTT_AGENT_DEPTH_LIMIT", 2),
		ProcessLimit:    envInt("MTT_PROCESS_LIMIT", 8),
		CreatedAt:       time.Now(),
	})
	if err != nil {
		return err
	}
	return database.Instances().Save(ctx, instance.Spec())
}

func refresh(ctx context.Context, models *gateway.Gateway, database store.Store, spec atom.ProviderSpec) {
	if spec.ModelListURL == "" {
		return
	}
	if modelsRefreshed, err := models.Refresh(ctx, spec.Name); err == nil {
		_ = database.Providers().SaveModels(ctx, spec.Name, modelsRefreshed)
		log.Printf("mtt: the provider %s gives %d models", spec.Name, len(modelsRefreshed))
	} else {
		log.Printf("mtt: the provider refresh is not complete: %v", err)
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

func env(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return value
}

func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		os.Setenv(key, strings.Trim(value, `"'`))
	}
}

func defaultModel(models *gateway.Gateway, providerName string) string {
	for _, provider := range models.Providers() {
		if provider.Name() != providerName {
			continue
		}
		for _, model := range provider.Models() {
			return model.ID
		}
	}
	return ""
}
