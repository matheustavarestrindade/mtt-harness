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
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestAPIFlow(test *testing.T) {
	database := memory.New()
	harnessRuntime := harness.New()
	bus := eventbus.New(harnessRuntime)
	toolRegistry := registry.New(harnessRuntime)
	broker := permission.NewBroker()
	models := gateway.New(harnessRuntime)
	testProvider := provider.NewTest("test", provider.TextWithUsage("hello", atom.Usage{Input: 10, Output: 2}))
	testutil.RequireNoError(test, models.Add(testProvider))

	instanceManager := instances.New(func(instanceID string) instances.SessionManager {
		return nil
	}, nil)
	runner := loop.New(harnessRuntime, loop.Config{
		Gateway:   models,
		Registry:  toolRegistry,
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

	response := request(test, server.URL+"/instances", "", "GET", nil)
	if response.StatusCode != http.StatusUnauthorized {
		test.Fatalf("status without token = %d", response.StatusCode)
	}

	var instance atom.InstanceSpec
	response = request(test, server.URL+"/instances", "secret", "POST", map[string]any{
		"workspace":     test.TempDir(),
		"default_model": "test-model",
	})
	if response.StatusCode != http.StatusCreated {
		test.Fatalf("instance status = %d", response.StatusCode)
	}
	decode(test, response, &instance)

	var session atom.Session
	response = request(test, server.URL+"/instances/"+instance.ID+"/sessions", "secret", "POST", nil)
	if response.StatusCode != http.StatusCreated {
		test.Fatalf("session status = %d", response.StatusCode)
	}
	decode(test, response, &session)

	testProvider.SetDelay(300 * time.Millisecond)
	response = request(test, server.URL+"/sessions/"+string(session.ID)+"/messages", "secret", "POST", map[string]any{"content": "hi"})
	if response.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(response.Body)
		test.Fatalf("message status = %d: %s", response.StatusCode, body)
	}
	var queued struct {
		Status  string `json:"status"`
		Message struct {
			ID string `json:"ID"`
		} `json:"message"`
	}
	response = request(test, server.URL+"/sessions/"+string(session.ID)+"/messages", "secret", "POST", map[string]any{"content": "cancel me"})
	if response.StatusCode != http.StatusAccepted {
		test.Fatalf("the second message status = %d", response.StatusCode)
	}
	decode(test, response, &queued)
	if queued.Status != "queued" || queued.Message.ID == "" {
		test.Fatalf("the second message = %+v", queued)
	}
	response = request(test, server.URL+"/sessions/"+string(session.ID)+"/queue/"+queued.Message.ID, "secret", "DELETE", nil)
	if response.StatusCode != http.StatusOK {
		test.Fatalf("the queue cancel status = %d", response.StatusCode)
	}
	testProvider.SetDelay(0)
	var messages []atom.Message
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		response = request(test, server.URL+"/sessions/"+string(session.ID)+"/messages", "secret", "GET", nil)
		decode(test, response, &messages)
		if len(messages) >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(messages) != 2 || messages[1].Content[0].Text != "hello" {
		test.Fatalf("the messages = %+v", messages)
	}
	if messages[0].Content[0].Text != "hi" {
		test.Fatalf("the removed message ran: %q", messages[0].Content[0].Text)
	}

	var statistics atom.Statistics
	response = request(test, server.URL+"/sessions/"+string(session.ID)+"/statistics", "secret", "GET", nil)
	decode(test, response, &statistics)
	if statistics.Calls != 1 || statistics.Input != 10 {
		test.Fatalf("statistics = %+v", statistics)
	}

	if _, _, operationError := sendAndWait(test, server.URL, string(session.ID), "again", 4); operationError != nil {
		test.Fatal(operationError)
	}
	var reverted struct {
		Status  string `json:"status"`
		Removed int    `json:"removed"`
	}
	response = request(test, server.URL+"/sessions/"+string(session.ID)+"/revert", "secret", "POST", map[string]any{"message_id": messages[0].ID})
	decode(test, response, &reverted)
	if reverted.Removed != 3 {
		test.Fatalf("the revert = %+v", reverted)
	}
	response = request(test, server.URL+"/sessions/"+string(session.ID)+"/messages", "secret", "GET", nil)
	decode(test, response, &messages)
	if len(messages) != 1 {
		test.Fatalf("the messages after the revert = %d", len(messages))
	}
	if _, _, operationError := sendAndWait(test, server.URL, string(session.ID), "after the revert", 2); operationError != nil {
		test.Fatal(operationError)
	}

	var modelList []atom.ModelInfo
	response = request(test, server.URL+"/instances/"+instance.ID+"/models", "secret", "GET", nil)
	decode(test, response, &modelList)
	if len(modelList) != 1 || modelList[0].ID != "test/test-model" {
		test.Fatalf("models = %+v", modelList)
	}

	response = request(test, server.URL+"/settings/agent_depth_limit", "secret", "PUT", map[string]any{"value": "5"})
	if response.StatusCode != http.StatusOK {
		test.Fatalf("global setting status = %d", response.StatusCode)
	}
	var settings map[string]string
	response = request(test, server.URL+"/settings", "secret", "GET", nil)
	decode(test, response, &settings)
	if settings["agent_depth_limit"] != "5" {
		test.Fatalf("global settings = %+v", settings)
	}
	response = request(test, server.URL+"/instances/"+instance.ID+"/settings/process_limit", "secret", "PUT", map[string]any{"value": "3"})
	if response.StatusCode != http.StatusOK {
		test.Fatalf("instance setting status = %d", response.StatusCode)
	}
	response = request(test, server.URL+"/instances/"+instance.ID+"/settings", "secret", "GET", nil)
	decode(test, response, &settings)
	if settings["process_limit"] != "3" {
		test.Fatalf("instance settings = %+v", settings)
	}
	response = request(test, server.URL+"/providers/openai/key", "secret", "PUT", map[string]any{"key": "sk-test"})
	if response.StatusCode != http.StatusOK {
		test.Fatalf("provider key status = %d", response.StatusCode)
	}
	response = request(test, server.URL+"/instances/"+instance.ID+"/providers/openai/key", "secret", "PUT", map[string]any{"key": "sk-instance"})
	if response.StatusCode != http.StatusOK {
		test.Fatalf("instance key status = %d", response.StatusCode)
	}
	key, operationError := database.Secrets().ResolveKey(context.Background(), instance.ID, "openai")
	if operationError != nil || key != "sk-instance" {
		test.Fatalf("resolved key = %q err = %v", key, operationError)
	}
	key, operationError = database.Secrets().ResolveKey(context.Background(), "other", "openai")
	if operationError != nil || key != "sk-test" {
		test.Fatalf("global key = %q err = %v", key, operationError)
	}
}

func request(test *testing.T, url string, token string, method string, body any) *http.Response {
	test.Helper()
	reader := bytes.NewReader(nil)
	if body != nil {
		data, operationError := json.Marshal(body)
		testutil.RequireNoError(test, operationError)

		reader = bytes.NewReader(data)
	}
	request, operationError := http.NewRequest(method, url, reader)
	testutil.RequireNoError(test, operationError)

	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	request.Header.Set("Content-Type", "application/json")
	response, operationError := http.DefaultClient.Do(request)
	testutil.RequireNoError(test, operationError)

	return response
}

func decode(test *testing.T, response *http.Response, value any) {
	test.Helper()
	defer response.Body.Close()
	testutil.RequireNoError(test, json.NewDecoder(response.Body).Decode(value))
}

func sendAndWait(test *testing.T, url string, session string, content string, expected int) ([]atom.Message, int, error) {
	test.Helper()
	response := request(test, url+"/sessions/"+session+"/messages", "secret", "POST", map[string]any{"content": content})
	if response.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(response.Body)
		test.Fatalf("message status = %d: %s", response.StatusCode, body)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		response = request(test, url+"/sessions/"+session+"/messages", "secret", "GET", nil)
		var messages []atom.Message
		decode(test, response, &messages)
		if len(messages) >= expected {
			return messages, len(messages), nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil, 0, fmt.Errorf("the messages did not arrive")
}
