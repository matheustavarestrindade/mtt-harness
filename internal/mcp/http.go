package mcp

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type httpTransport struct {
	client          *http.Client
	url             string
	headers         map[string]string
	mutex           sync.Mutex
	sessionID       string
	protocolVersion string
	replies         chan []byte
	failures        chan error
	closed          chan struct{}
	closeOnce       sync.Once
	lifecycle       context.Context
	cancel          context.CancelFunc
}

func startHTTPTransport(specification ServerSpec) (*httpTransport, error) {
	lifecycle, cancel := context.WithCancel(context.Background())
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 30 * time.Second
	return &httpTransport{client: &http.Client{Transport: transport}, url: specification.URL, headers: specification.Headers, protocolVersion: ProtocolVersion, replies: make(chan []byte, 32), failures: make(chan error, 1), closed: make(chan struct{}), lifecycle: lifecycle, cancel: cancel}, nil
}

func (transport *httpTransport) SetProtocolVersion(version string) {
	transport.mutex.Lock()
	defer transport.mutex.Unlock()
	transport.protocolVersion = version
}

func (transport *httpTransport) newHTTPRequest(operationContext context.Context, method string, body io.Reader) (*http.Request, error) {
	request, operationError := http.NewRequestWithContext(operationContext, method, transport.url, body)
	if operationError != nil {
		return nil, operationError
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	for key, value := range transport.headers {
		request.Header.Set(key, value)
	}
	transport.mutex.Lock()
	sessionID, version := transport.sessionID, transport.protocolVersion
	transport.mutex.Unlock()
	request.Header.Set("MCP-Protocol-Version", version)
	if sessionID != "" {
		request.Header.Set("Mcp-Session-Id", sessionID)
	}
	return request, nil
}

func (transport *httpTransport) Send(operationContext context.Context, data []byte) error {
	select {
	case <-transport.closed:
		return io.EOF
	default:
	}
	request, operationError := transport.newHTTPRequest(operationContext, http.MethodPost, bytes.NewReader(data))
	if operationError != nil {
		return operationError
	}
	response, operationError := transport.client.Do(request)
	if operationError != nil {
		return operationError
	}
	if identifier := response.Header.Get("Mcp-Session-Id"); identifier != "" {
		transport.mutex.Lock()
		transport.sessionID = identifier
		transport.mutex.Unlock()
	}
	if response.StatusCode >= 400 {
		defer response.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("MCP HTTP status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	if strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		// Return immediately so call can consume its response while the server
		// keeps the SSE connection open. The request context owns this reader.
		go func() {
			defer response.Body.Close()
			if operationError := transport.readServerSentMessages(operationContext, response.Body); operationError != nil && operationContext.Err() == nil {
				select {
				case transport.failures <- operationError:
				default:
				}
			}
		}()
		return nil
	}
	defer response.Body.Close()
	body, operationError := io.ReadAll(io.LimitReader(response.Body, 16*1024*1024+1))
	if operationError != nil {
		return operationError
	}
	if len(body) > 16*1024*1024 {
		return fmt.Errorf("MCP response exceeds 16 MiB")
	}
	if len(bytes.TrimSpace(body)) > 0 {
		transport.enqueueServerMessage(operationContext, body)
	}
	return nil
}

func (transport *httpTransport) readServerSentMessages(operationContext context.Context, body io.Reader) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var data strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			if data.Len() > 16*1024*1024 {
				return fmt.Errorf("MCP event exceeds 16 MiB")
			}
			continue
		}
		if strings.TrimSpace(line) == "" && data.Len() > 0 {
			transport.enqueueServerMessage(operationContext, []byte(data.String()))
			data.Reset()
		}
	}
	if data.Len() > 0 {
		transport.enqueueServerMessage(operationContext, []byte(data.String()))
	}
	return scanner.Err()
}

func (transport *httpTransport) enqueueServerMessage(operationContext context.Context, data []byte) {
	select {
	case transport.replies <- data:
	case <-transport.closed:
	case <-operationContext.Done():
	}
}

func (transport *httpTransport) Receive(operationContext context.Context) ([]byte, error) {
	select {
	case data := <-transport.replies:
		return data, nil
	case operationError := <-transport.failures:
		return nil, operationError
	case <-transport.closed:
		return nil, io.EOF
	case <-operationContext.Done():
		return nil, operationContext.Err()
	}
}

// The optional GET stream carries unsolicited catalog changes. Reconnecting
// triggers a fresh tools/list, covering notifications lost during the gap.
func (transport *httpTransport) StartNotifications() {
	go func() {
		for transport.lifecycle.Err() == nil {
			request, operationError := transport.newHTTPRequest(transport.lifecycle, http.MethodGet, nil)
			if operationError != nil {
				return
			}
			response, operationError := transport.client.Do(request)
			if operationError == nil {
				if response.StatusCode == http.StatusMethodNotAllowed {
					response.Body.Close()
					return
				}
				if response.StatusCode == http.StatusOK && strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
					transport.enqueueServerMessage(transport.lifecycle, []byte(`{"jsonrpc":"2.0","method":"notifications/tools/list_changed"}`))
					_ = transport.readServerSentMessages(transport.lifecycle, response.Body)
				}
				response.Body.Close()
			}
			timer := time.NewTimer(time.Second)
			select {
			case <-timer.C:
			case <-transport.lifecycle.Done():
				timer.Stop()
				return
			}
		}
	}()
}

func (transport *httpTransport) Close() error {
	transport.closeOnce.Do(func() {
		transport.cancel()
		close(transport.closed)
		transport.mutex.Lock()
		session := transport.sessionID
		transport.mutex.Unlock()
		if session != "" {
			operationContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if request, operationError := transport.newHTTPRequest(operationContext, http.MethodDelete, nil); operationError == nil {
				if response, operationError := transport.client.Do(request); operationError == nil {
					response.Body.Close()
				}
			}
		}
		transport.client.CloseIdleConnections()
	})
	return nil
}
