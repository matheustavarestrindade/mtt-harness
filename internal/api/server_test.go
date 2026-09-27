package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/api"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/eventbus"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/permission"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/registry"
)

func TestAPIFlow(t *testing.T) {
	database := memory.New()
	bus := eventbus.New()
	h := harness.New()
	reg := registry.New()
	broker := permission.NewBroker()
	models := gateway.New()
	if err := models.Add(provider.NewTest("test", provider.TextWithUsage("hello", atom.Usage{Input: 10, Output: 2}))); err != nil {
		t.Fatal(err)
	}
	instanceManager := instances.New(func(instanceID string) instances.SessionManager { return nil }, nil)
	runner := loop.New(h, loop.Config{
		Gateway:   models,
		Registry:  reg,
		Store:     database,
		Bus:       bus,
		Broker:    broker,
		Engine:    permission.NewEngine(),
		Instances: instanceManager,
	})
	server := httptest.NewServer(api.New(api.Config{
		Token:     "secret",
		Store:     database,
		Instances: instanceManager,
		Bus:       bus,
		Broker:    broker,
		Loop:      runner,
		Gateway:   models,
	}).Handler())
	defer server.Close()

	response := request(t, server.URL+"/instances", "", "GET", nil)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status without token = %d", response.StatusCode)
	}

	var instance atom.InstanceSpec
	response = request(t, server.URL+"/instances", "secret", "POST", map[string]any{
		"workspace":     t.TempDir(),
		"default_model": "test-model",
	})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("instance status = %d", response.StatusCode)
	}
	decode(t, response, &instance)

	var session atom.Session
	response = request(t, server.URL+"/instances/"+instance.ID+"/sessions", "secret", "POST", nil)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("session status = %d", response.StatusCode)
	}
	decode(t, response, &session)

	var answer struct {
		Session string `json:"session"`
		Message *struct {
			Role    string `json:"Role"`
			Content []struct {
				Text string `json:"Text"`
			} `json:"Content"`
		} `json:"message"`
	}
	response = request(t, server.URL+"/sessions/"+string(session.ID)+"/messages", "secret", "POST", map[string]any{"content": "hi"})
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("message status = %d: %s", response.StatusCode, body)
	}
	decode(t, response, &answer)
	if answer.Message == nil || answer.Message.Content[0].Text != "hello" {
		t.Fatalf("answer = %+v", answer)
	}

	var statistics atom.Statistics
	response = request(t, server.URL+"/sessions/"+string(session.ID)+"/statistics", "secret", "GET", nil)
	decode(t, response, &statistics)
	if statistics.Calls != 1 || statistics.Input != 10 {
		t.Fatalf("statistics = %+v", statistics)
	}

	var modelList []atom.ModelInfo
	response = request(t, server.URL+"/instances/"+instance.ID+"/models", "secret", "GET", nil)
	decode(t, response, &modelList)
	if len(modelList) != 1 || modelList[0].ID != "test-model" {
		t.Fatalf("models = %+v", modelList)
	}

	response = request(t, server.URL+"/settings/agent_depth_limit", "secret", "PUT", map[string]any{"value": "5"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("global setting status = %d", response.StatusCode)
	}
	var settings map[string]string
	response = request(t, server.URL+"/settings", "secret", "GET", nil)
	decode(t, response, &settings)
	if settings["agent_depth_limit"] != "5" {
		t.Fatalf("global settings = %+v", settings)
	}
	response = request(t, server.URL+"/instances/"+instance.ID+"/settings/process_limit", "secret", "PUT", map[string]any{"value": "3"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("instance setting status = %d", response.StatusCode)
	}
	response = request(t, server.URL+"/instances/"+instance.ID+"/settings", "secret", "GET", nil)
	decode(t, response, &settings)
	if settings["process_limit"] != "3" {
		t.Fatalf("instance settings = %+v", settings)
	}
	response = request(t, server.URL+"/providers/openai/key", "secret", "PUT", map[string]any{"key": "sk-test"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("provider key status = %d", response.StatusCode)
	}
	response = request(t, server.URL+"/instances/"+instance.ID+"/providers/openai/key", "secret", "PUT", map[string]any{"key": "sk-instance"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("instance key status = %d", response.StatusCode)
	}
	key, err := database.Secrets().ResolveKey(context.Background(), instance.ID, "openai")
	if err != nil || key != "sk-instance" {
		t.Fatalf("resolved key = %q err = %v", key, err)
	}
	key, err = database.Secrets().ResolveKey(context.Background(), "other", "openai")
	if err != nil || key != "sk-test" {
		t.Fatalf("global key = %q err = %v", key, err)
	}
}

func request(t *testing.T, url string, token string, method string, body any) *http.Response {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func decode(t *testing.T, response *http.Response, value any) {
	t.Helper()
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(value); err != nil {
		t.Fatal(err)
	}
}
