package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestResponsesCallAssemblyAndFailureMetadataAcrossTransports(testingHandle *testing.T) {
	for _, transport := range []string{"http", "websocket"} {
		testingHandle.Run(transport, func(testingHandle *testing.T) {
			operationContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			finish := make(chan struct{})
			var release sync.Once
			defer release.Do(func() { close(finish) })
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				requests.Add(1)
				var send func(string) bool
				if transport == "websocket" {
					connection, operationError := websocket.Accept(responseWriter, request, nil)
					if operationError != nil {
						return
					}
					defer connection.CloseNow()
					if _, _, operationError = connection.Read(request.Context()); operationError != nil {
						return
					}
					send = func(event string) bool {
						return connection.Write(request.Context(), websocket.MessageText, []byte(event)) == nil
					}
				} else {
					responseWriter.Header().Set("Content-Type", "text/event-stream")
					send = func(event string) bool {
						_, operationError := fmt.Fprintf(responseWriter, "data: %s\n\n", event)
						responseWriter.(http.Flusher).Flush()
						return operationError == nil
					}
				}
				for _, event := range []string{
					`{"type":"response.created","response":{"id":"transport-response"}}`,
					`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"one","call_id":"call-one","name":"inspect","status":"in_progress"}}`,
					`{"type":"response.function_call_arguments.delta","output_index":0,"item_id":"one","delta":"{}"}`,
					`{"type":"response.function_call_arguments.done","output_index":0,"item_id":"one","arguments":"{}"}`,
					`{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"one"}}`,
				} {
					if !send(event) {
						return
					}
				}
				select {
				case <-finish:
				case <-request.Context().Done():
					return
				}
				for _, event := range []string{
					`{"type":"response.output_item.done","output_index":1,"item":{"type":"function_call","id":"two","call_id":"call-two","name":"write","arguments":"{","status":"incomplete"}}`,
					`{"type":"response.incomplete","response":{"id":"transport-response","incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":20,"output_tokens":9}}}`,
				} {
					if !send(event) {
						return
					}
				}
			}))
			defer server.Close()
			standardProvider := New(atom.ProviderSpec{Name: "synthetic", Protocol: "responses", APIURL: server.URL, Transport: transport})
			defer standardProvider.Close()
			defer cancel()
			stream, operationError := standardProvider.Stream(operationContext, atom.Request{Model: "synthetic-model", Messages: []atom.Message{{Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "Read a test item."}}}}})
			testutil.RequireNoError(testingHandle, operationError)
			part, operationError := stream.Recv(operationContext)
			testutil.RequireNoError(testingHandle, operationError)
			if part.ToolCall == nil || part.ToolCall.ID != "call-one" || string(part.ToolCall.Input) != "{}" {
				testingHandle.Fatalf("completed call did not arrive before response termination: %+v", part)
			}
			release.Do(func() { close(finish) })
			var usage *atom.Usage
			for {
				part, operationError = stream.Recv(operationContext)
				if operationError != nil {
					break
				}
				if part.ToolCall != nil {
					testingHandle.Fatal("unfinished/duplicate call was delivered")
				}
				if part.Usage != nil {
					usage = part.Usage
				}
			}
			if errors.Is(operationError, io.EOF) || operationError == nil || !strings.Contains(operationError.Error(), "max_output_tokens") || !strings.Contains(operationError.Error(), "transport-response") || !strings.Contains(operationError.Error(), "call-two") {
				testingHandle.Fatalf("lost terminal cause: %v", operationError)
			}
			if usage == nil || usage.Input != 20 || usage.Output != 9 {
				testingHandle.Fatalf("missing usage: %+v", usage)
			}
			if requests.Load() != 1 {
				testingHandle.Fatalf("accepted partial generation was retried %d times", requests.Load())
			}
		})
	}
}
