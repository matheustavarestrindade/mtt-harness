package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/api"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/plugins"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/registry"
	"github.com/matheustavarestrindade/mtt-harness/internal/tools"
	"github.com/matheustavarestrindade/mtt-harness/plugins/pathtools"
)

func main() {
	loadDotEnv(".env")

	port := env("MTT_PORT", "8080")
	token := env("MTT_API_TOKEN", "")
	workspace := env("MTT_WORKSPACE", ".")

	h := harness.New()
	reg := registry.New()
	for _, tool := range []harness.Tool{
		tools.Bash{},
		tools.Read{},
		tools.Write{},
		tools.NewSearch(reg),
		tools.Agent{},
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

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           api.New(token).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()

	log.Printf("mtt: the API is on port %s", port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("mtt: %v", err)
	}
}

func env(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
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
