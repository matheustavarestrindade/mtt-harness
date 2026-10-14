package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestResponsesWebSocketFallbackOnlyBeforeAcceptedRequest(test *testing.T) {
	for _, status := range []int{404, 405, 426, 501, 401, 403, 413, 429, 503} {
		test.Run(fmt.Sprint(status), func(test *testing.T) {
			var upgrades, posts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodGet {
					upgrades.Add(1)
					responseWriter.Header().Set("X-Request-ID", "upgrade-request")
					responseWriter.WriteHeader(status)
					fmt.Fprint(responseWriter, `{"error":{"code":"unavailable","message":"Endpoint rejected the upgrade"}}`)
					return
				}
				posts.Add(1)
				responseWriter.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(responseWriter, "data: {\"type\":\"response.completed\",\"response\":{\"output\":[]}}\n\n")
			}))
			test.Cleanup(server.Close)
			modelProvider := New(atom.ProviderSpec{Name: "compatible", Protocol: "responses", APIURL: server.URL, Transport: "auto"})
			test.Cleanup(func() { modelProvider.Close() })
			operationContext := transportTestContext(test, "workspace", "session")
			stream, operationError := modelProvider.Stream(operationContext, atom.Request{Model: "transport-model"})
			fallback := status == 404 || status == 405 || status == 426 || status == 501
			if !fallback {
				if operationError == nil || !strings.Contains(operationError.Error(), "upgrade-request") || posts.Load() != 0 {
					test.Fatalf("rejected upgrade was hidden/retried: %v posts=%d", operationError, posts.Load())
				}
				return
			}
			testutil.RequireNoError(test, operationError)
			_, operationError = collectTransportMessage(operationContext, stream)
			testutil.RequireNoError(test, operationError)
			stream, operationError = modelProvider.Stream(operationContext, atom.Request{Model: "transport-model"})
			testutil.RequireNoError(test, operationError)
			_, operationError = collectTransportMessage(operationContext, stream)
			testutil.RequireNoError(test, operationError)
			if upgrades.Load() != 1 || posts.Load() != 2 {
				test.Fatalf("fallback was not session-scoped: upgrades=%d posts=%d", upgrades.Load(), posts.Load())
			}
		})
	}
}

func TestResponsesWebSocketRecoversMissingPreviousResponseWithFullContext(test *testing.T) {
	operationContext := transportTestContext(test, "workspace", "session")
	requests := make(chan map[string]any, 3)
	var upgrades atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		connection, operationError := websocket.Accept(responseWriter, request, nil)
		if operationError != nil {
			return
		}
		defer connection.CloseNow()
		number := upgrades.Add(1)
		payload, operationError := readWebSocketRequest(operationContext, connection)
		if operationError != nil {
			return
		}
		requests <- payload
		if completeWebSocketResponse(operationContext, connection, "first", "Ready") != nil {
			return
		}
		if number != 1 {
			return
		}
		payload, operationError = readWebSocketRequest(operationContext, connection)
		if operationError != nil {
			return
		}
		requests <- payload
		writeWebSocketEvent(operationContext, connection, map[string]any{"type": "error", "status": 400, "error": map[string]any{"type": "invalid_request_error", "code": "previous_response_not_found", "message": "The prior response is no longer cached"}})
		connection.Read(operationContext)
	}))
	test.Cleanup(server.Close)
	modelProvider := New(atom.ProviderSpec{Name: "compatible", Protocol: "responses", APIURL: server.URL, Transport: "websocket"})
	test.Cleanup(func() { modelProvider.Close() })
	request := atom.Request{Model: "transport-model", Messages: []atom.Message{{Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "Start"}}}}}
	stream, operationError := modelProvider.Stream(operationContext, request)
	testutil.RequireNoError(test, operationError)
	message, operationError := collectTransportMessage(operationContext, stream)
	testutil.RequireNoError(test, operationError)
	request.Messages = append(request.Messages, message, atom.Message{Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "Next"}}})
	stream, operationError = modelProvider.Stream(operationContext, request)
	testutil.RequireNoError(test, operationError)
	message, operationError = collectTransportMessage(operationContext, stream)
	testutil.RequireNoError(test, operationError)
	<-requests
	incremental, recovered := <-requests, <-requests
	if upgrades.Load() != 2 || incremental["previous_response_id"] != "first" || recovered["previous_response_id"] != nil || len(recovered["input"].([]any)) != 3 || concatenateContentText(message.Content) != "Ready" {
		test.Fatalf("invalid recovery: upgrades=%d incremental=%v recovered=%v response=%#v", upgrades.Load(), incremental, recovered, message)
	}
}

func TestResponsesWebSocketDoesNotRepeatPartialGeneration(test *testing.T) {
	for _, terminal := range []string{"disconnect", "failed", "missing-after-created"} {
		test.Run(terminal, func(test *testing.T) {
			operationContext := transportTestContext(test, "workspace", "session")
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodGet {
					requests.Add(100)
					http.Error(responseWriter, "unexpected fallback", 500)
					return
				}
				connection, operationError := websocket.Accept(responseWriter, request, nil)
				if operationError != nil {
					return
				}
				defer connection.CloseNow()
				if _, operationError := readWebSocketRequest(operationContext, connection); operationError != nil {
					return
				}
				requests.Add(1)
				writeWebSocketEvent(operationContext, connection, map[string]any{"type": "response.created", "response": map[string]any{"id": "partial"}})
				writeWebSocketEvent(operationContext, connection, map[string]any{"type": "response.output_text.delta", "delta": "Retain this partial response"})
				switch terminal {
				case "failed":
					writeWebSocketEvent(operationContext, connection, map[string]any{"type": "response.failed", "response": map[string]any{"id": "partial", "error": map[string]any{"code": "server_error", "message": "Provider interrupted generation"}, "usage": map[string]any{"input_tokens": 70, "output_tokens": 5, "cost": 0.1}}})
				case "missing-after-created":
					writeWebSocketEvent(operationContext, connection, map[string]any{"type": "error", "status": 400, "error": map[string]any{"code": "previous_response_not_found", "message": "Not a safe continuation retry"}})
				}
			}))
			test.Cleanup(server.Close)
			modelProvider := New(atom.ProviderSpec{Name: "compatible", Protocol: "responses", APIURL: server.URL, Transport: "auto"})
			test.Cleanup(func() { modelProvider.Close() })
			stream, operationError := modelProvider.Stream(operationContext, atom.Request{Model: "transport-model"})
			testutil.RequireNoError(test, operationError)
			message, operationError := collectTransportMessage(operationContext, stream)
			if operationError == nil || requests.Load() != 1 || concatenateContentText(message.Content) != "Retain this partial response" {
				test.Fatalf("partial response was lost/retried: %v %#v calls=%d", operationError, message, requests.Load())
			}
			if terminal == "failed" && (message.Usage == nil || message.Usage.Input != 70 || message.Usage.Cost == nil || message.Usage.Cost.Value != 0.1) {
				test.Fatalf("failed response lost billable usage: %#v", message.Usage)
			}
		})
	}
}

func TestResponsesWebSocketCancellationAndCloseJoinReaders(test *testing.T) {
	for _, shutdown := range []bool{false, true} {
		test.Run(fmt.Sprint(shutdown), func(test *testing.T) {
			started := make(chan struct{})
			serverStopped := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				connection, operationError := websocket.Accept(responseWriter, request, nil)
				if operationError != nil {
					return
				}
				defer connection.CloseNow()
				defer close(serverStopped)
				if _, operationError = readWebSocketRequest(context.Background(), connection); operationError != nil {
					return
				}
				close(started)
				connection.Read(context.Background())
			}))
			test.Cleanup(server.Close)
			modelProvider := New(atom.ProviderSpec{Name: "compatible", Protocol: "responses", APIURL: server.URL, Transport: "websocket"})
			test.Cleanup(func() { modelProvider.Close() })
			operationContext := transportTestContext(test, "workspace", "session")
			requestContext, cancel := context.WithCancel(operationContext)
			defer cancel()
			stream, operationError := modelProvider.Stream(requestContext, atom.Request{Model: "transport-model"})
			testutil.RequireNoError(test, operationError)
			select {
			case <-started:
			case <-operationContext.Done():
				test.Fatal("request did not start")
			}
			// A cancelled waiter on the same session must not cancel its owner.
			waitContext, stopWaiting := context.WithTimeout(operationContext, 20*time.Millisecond)
			defer stopWaiting()
			_, waitError := modelProvider.Stream(waitContext, atom.Request{Model: "transport-model"})
			if !errors.Is(waitError, context.DeadlineExceeded) {
				test.Fatalf("uncancellable session gate: %v", waitError)
			}
			select {
			case <-serverStopped:
				test.Fatal("waiting request stopped the active owner")
			default:
			}
			if shutdown {
				testutil.RequireNoError(test, modelProvider.Close())
			} else {
				cancel()
			}
			_, operationError = collectTransportMessage(operationContext, stream)
			if !errors.Is(operationError, context.Canceled) {
				test.Fatalf("cancellation cause was lost: %v", operationError)
			}
			select {
			case <-serverStopped:
			case <-operationContext.Done():
				test.Fatal("provider did not close the socket")
			}
			modelProvider.Close()
			if _, operationError := modelProvider.Stream(harness.WithSession(context.Background(), atom.Session{ID: "later"}), atom.Request{Model: "transport-model"}); operationError == nil {
				test.Fatal("closed provider accepted a new socket")
			}
		})
	}
}
