package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func TestResponsesConnectionAdmissionIsBoundedAndCancellable(test *testing.T) {
	pool := newResponsesWebsocketPool()
	var leases []*responsesWebsocketLease
	defer func() {
		for _, lease := range leases {
			lease.release()
		}
		pool.Close()
	}()
	for index := 0; index < maximumResponsesConnections; index++ {
		lease, operationError := pool.acquire(context.Background(), responsesConnectionScope{instanceID: "workspace", sessionID: fmt.Sprint(index)})
		if operationError != nil {
			test.Fatal(operationError)
		}
		leases = append(leases, lease)
	}
	if _, operationError := pool.acquire(context.Background(), responsesConnectionScope{instanceID: "workspace", sessionID: "overflow"}); operationError == nil {
		test.Fatal("active connection limit was unbounded")
	}
	pool.evictIdleConnections(time.Now().Add(2 * responsesConnectionIdleLimit))
	pool.mutex.Lock()
	remaining := len(pool.entries)
	pool.mutex.Unlock()
	if remaining != maximumResponsesConnections {
		test.Fatal("idle sweep evicted an active owner")
	}
	leases[0].release()
	lease, operationError := pool.acquire(context.Background(), responsesConnectionScope{instanceID: "workspace", sessionID: "replacement"})
	if operationError != nil {
		test.Fatalf("unused connection was not evicted: %v", operationError)
	}
	leases = append(leases, lease)
	pool.mutex.Lock()
	remaining = len(pool.entries)
	pool.mutex.Unlock()
	if remaining != maximumResponsesConnections {
		test.Fatal("replacement exceeded the pool limit")
	}
}

func TestResponsesProviderCloseJoinsHTTPFallback(test *testing.T) {
	started := make(chan struct{})
	stopped := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			responseWriter.WriteHeader(http.StatusUpgradeRequired)
			return
		}
		defer close(stopped)
		responseWriter.Header().Set("Content-Type", "text/event-stream")
		responseWriter.WriteHeader(http.StatusOK)
		responseWriter.(http.Flusher).Flush()
		close(started)
		<-request.Context().Done()
	}))
	defer server.Close()
	modelProvider := New(atom.ProviderSpec{Name: "compatible", Protocol: "responses", APIURL: server.URL, Transport: "auto"})
	defer modelProvider.Close()
	operationContext := transportTestContext(test, "workspace", "session")
	stream, operationError := modelProvider.Stream(operationContext, atom.Request{Model: "transport-model"})
	if operationError != nil {
		test.Fatal(operationError)
	}
	<-started
	modelProvider.Close()
	_, operationError = collectTransportMessage(operationContext, stream)
	if !errors.Is(operationError, context.Canceled) {
		test.Fatalf("fallback escaped provider shutdown: %v", operationError)
	}
	select {
	case <-stopped:
	case <-operationContext.Done():
		test.Fatal("fallback HTTP request did not stop")
	}
}

func TestResponsesProviderCloseCancelsCredentialResolution(test *testing.T) {
	modelProvider := New(atom.ProviderSpec{Name: "compatible", Protocol: "responses", APIURL: "http://unreachable.invalid", Transport: "websocket"})
	entered := make(chan struct{})
	finished := make(chan error, 1)
	modelProvider.SetHeaderResolver(func(operationContext context.Context) (http.Header, error) {
		close(entered)
		<-operationContext.Done()
		return nil, operationContext.Err()
	})
	go func() {
		_, operationError := modelProvider.Stream(context.Background(), atom.Request{Model: "transport-model"})
		finished <- operationError
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		test.Fatal("credential resolution did not start")
	}
	var closers sync.WaitGroup
	for index := 0; index < 2; index++ {
		closers.Add(1)
		go func() { defer closers.Done(); modelProvider.Close() }()
	}
	select {
	case operationError := <-finished:
		if !errors.Is(operationError, context.Canceled) {
			test.Fatalf("credential resolution was not cancelled: %v", operationError)
		}
	case <-time.After(time.Second):
		test.Fatal("close did not join credential resolution")
	}
	closers.Wait()
}
