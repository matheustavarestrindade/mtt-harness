package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
	testProvider := provider.NewTest("test", provider.TextWithUsage("hello", atom.Usage{Input: 10, Output: 2}))
	if err := models.Add(testProvider); err != nil {
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
		Queue:     loop.NewQueue(runner),
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

	testProvider.SetDelay(300 * time.Millisecond)
	response = request(t, server.URL+"/sessions/"+string(session.ID)+"/messages", "secret", "POST", map[string]any{"content": "hi"})
	if response.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("message status = %d: %s", response.StatusCode, body)
	}
	var queued struct {
		Status  string `json:"status"`
		Message struct {
			ID string `json:"ID"`
		} `json:"message"`
	}
	response = request(t, server.URL+"/sessions/"+string(session.ID)+"/messages", "secret", "POST", map[string]any{"content": "cancel me"})
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("the second message status = %d", response.StatusCode)
	}
	decode(t, response, &queued)
	if queued.Status != "queued" || queued.Message.ID == "" {
		t.Fatalf("the second message = %+v", queued)
	}
	response = request(t, server.URL+"/sessions/"+string(session.ID)+"/queue/"+queued.Message.ID, "secret", "DELETE", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("the queue cancel status = %d", response.StatusCode)
	}
	testProvider.SetDelay(0)
	var messages []atom.Message
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		response = request(t, server.URL+"/sessions/"+string(session.ID)+"/messages", "secret", "GET", nil)
		decode(t, response, &messages)
		if len(messages) >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(messages) != 2 || messages[1].Content[0].Text != "hello" {
		t.Fatalf("the messages = %+v", messages)
	}
	if messages[0].Content[0].Text != "hi" {
		t.Fatalf("the removed message ran: %q", messages[0].Content[0].Text)
	}

	var statistics atom.Statistics
	response = request(t, server.URL+"/sessions/"+string(session.ID)+"/statistics", "secret", "GET", nil)
	decode(t, response, &statistics)
	if statistics.Calls != 1 || statistics.Input != 10 {
		t.Fatalf("statistics = %+v", statistics)
	}

	if _, _, err := sendAndWait(t, server.URL, string(session.ID), "again", 4); err != nil {
		t.Fatal(err)
	}
	var reverted struct {
		Status  string `json:"status"`
		Removed int    `json:"removed"`
	}
	response = request(t, server.URL+"/sessions/"+string(session.ID)+"/revert", "secret", "POST", map[string]any{"message_id": messages[0].ID})
	decode(t, response, &reverted)
	if reverted.Removed != 3 {
		t.Fatalf("the revert = %+v", reverted)
	}
	response = request(t, server.URL+"/sessions/"+string(session.ID)+"/messages", "secret", "GET", nil)
	decode(t, response, &messages)
	if len(messages) != 1 {
		t.Fatalf("the messages after the revert = %d", len(messages))
	}
	if _, _, err := sendAndWait(t, server.URL, string(session.ID), "after the revert", 2); err != nil {
		t.Fatal(err)
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

func sendAndWait(t *testing.T, url string, session string, content string, want int) ([]atom.Message, int, error) {
	t.Helper()
	response := request(t, url+"/sessions/"+session+"/messages", "secret", "POST", map[string]any{"content": content})
	if response.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("message status = %d: %s", response.StatusCode, body)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		response = request(t, url+"/sessions/"+session+"/messages", "secret", "GET", nil)
		var messages []atom.Message
		decode(t, response, &messages)
		if len(messages) >= want {
			return messages, len(messages), nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil, 0, fmt.Errorf("the messages did not arrive")
}
