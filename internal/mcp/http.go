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
	client    *http.Client
	url       string
	headers   map[string]string
	mu        sync.Mutex
	sessionID string
	replies   chan []byte
	closed    chan struct{}
	closeOnce sync.Once
}

func startHTTP(spec ServerSpec) (*httpTransport, error) {
	return &httpTransport{
		client:  &http.Client{Timeout: 60 * time.Second},
		url:     spec.URL,
		headers: spec.Headers,
		replies: make(chan []byte, 32),
		closed:  make(chan struct{}),
	}, nil
}

func (t *httpTransport) Send(ctx context.Context, data []byte) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	for key, value := range t.headers {
		request.Header.Set(key, value)
	}
	t.mu.Lock()
	session := t.sessionID
	t.mu.Unlock()
	if session != "" {
		request.Header.Set("Mcp-Session-Id", session)
	}
	response, err := t.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if id := response.Header.Get("Mcp-Session-Id"); id != "" {
		t.mu.Lock()
		t.sessionID = id
		t.mu.Unlock()
	}
	if response.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("mcp: the server gives the status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	if strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		return t.readSSE(response.Body)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 16*1024*1024))
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(body)) > 0 {
		t.push(body)
	}
	return nil
}

func (t *httpTransport) readSSE(body io.Reader) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var data strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			continue
		}
		if strings.TrimSpace(line) == "" && data.Len() > 0 {
			t.push([]byte(data.String()))
			data.Reset()
		}
	}
	if data.Len() > 0 {
		t.push([]byte(data.String()))
	}
	return scanner.Err()
}

func (t *httpTransport) push(data []byte) {
	select {
	case t.replies <- data:
	case <-t.closed:
	}
}

func (t *httpTransport) Receive(ctx context.Context) ([]byte, error) {
	select {
	case data := <-t.replies:
		return data, nil
	case <-t.closed:
		return nil, io.EOF
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (t *httpTransport) Close() error {
	t.closeOnce.Do(func() { close(t.closed) })
	return nil
}
