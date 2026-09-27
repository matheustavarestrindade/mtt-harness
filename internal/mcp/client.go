package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

const ProtocolVersion = "2025-06-18"

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type Content struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
	URI      string `json:"uri"`
}

type CallResult struct {
	Content []Content
	IsError bool
}

type Transport interface {
	Send(ctx context.Context, data []byte) error
	Receive(ctx context.Context) ([]byte, error)
	Close() error
}

type Client struct {
	name      string
	transport Transport
	timeout   time.Duration
	mu        sync.Mutex
	next      int64
	pending   map[int64]chan rpcMessage
	closed    bool
	onChanged func(ctx context.Context)
}

func Start(ctx context.Context, spec ServerSpec, timeout time.Duration) (*Client, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	var transport Transport
	var err error
	switch {
	case spec.Command != "":
		transport, err = startStdio(spec)
	case spec.URL != "":
		transport, err = startHTTP(spec)
	default:
		return nil, errors.New("mcp: the server data has no command and no URL")
	}
	if err != nil {
		return nil, err
	}
	client := &Client{
		name:      spec.Name,
		transport: transport,
		timeout:   timeout,
		pending:   map[int64]chan rpcMessage{},
	}
	go client.read()
	initContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if _, err := client.call(initContext, "initialize", map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "mtt-harness", "version": "0.1.0"},
	}); err != nil {
		_ = transport.Close()
		return nil, fmt.Errorf("mcp: the initialize of the server %s: %w", spec.Name, err)
	}
	if err := client.notify(ctx, "notifications/initialized", nil); err != nil {
		_ = transport.Close()
		return nil, err
	}
	return client, nil
}

func (c *Client) Name() string {
	return c.name
}

func (c *Client) OnToolsChanged(handler func(ctx context.Context)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onChanged = handler
}

func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errors.New("mcp: the connection is closed")
	}
	c.next++
	id := c.next
	channel := make(chan rpcMessage, 1)
	c.pending[id] = channel
	c.mu.Unlock()
	data, err := json.Marshal(rpcMessage{JSONRPC: "2.0", ID: &id, Method: method, Params: rawJSON(params)})
	if err != nil {
		c.discard(id)
		return nil, err
	}
	if err := c.transport.Send(ctx, data); err != nil {
		c.discard(id)
		return nil, err
	}
	timer := time.NewTimer(c.timeout)
	defer timer.Stop()
	select {
	case message, ok := <-channel:
		if !ok {
			return nil, errors.New("mcp: the connection is closed")
		}
		if message.Error != nil {
			return nil, fmt.Errorf("mcp: %s", message.Error.Message)
		}
		return message.Result, nil
	case <-timer.C:
		c.discard(id)
		return nil, errors.New("mcp: the request timeout elapsed")
	case <-ctx.Done():
		c.discard(id)
		return nil, ctx.Err()
	}
}

func (c *Client) notify(ctx context.Context, method string, params any) error {
	data, err := json.Marshal(rpcMessage{JSONRPC: "2.0", Method: method, Params: rawJSON(params)})
	if err != nil {
		return err
	}
	return c.transport.Send(ctx, data)
}

func (c *Client) discard(id int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pending, id)
}

func (c *Client) read() {
	for {
		data, err := c.transport.Receive(context.Background())
		if err != nil {
			c.fail()
			return
		}
		var message rpcMessage
		if err := json.Unmarshal(data, &message); err != nil {
			continue
		}
		if message.ID == nil {
			c.mu.Lock()
			handler := c.onChanged
			c.mu.Unlock()
			if message.Method == "notifications/tools/list_changed" && handler != nil {
				handler(context.Background())
			}
			continue
		}
		c.mu.Lock()
		channel, ok := c.pending[*message.ID]
		if ok {
			delete(c.pending, *message.ID)
		}
		c.mu.Unlock()
		if ok {
			channel <- message
		}
	}
}

func (c *Client) fail() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	for id, channel := range c.pending {
		close(channel)
		delete(c.pending, id)
	}
}

func (c *Client) Close() error {
	c.fail()
	return c.transport.Close()
}

func (c *Client) ListTools(ctx context.Context) ([]ToolSpec, error) {
	result, err := c.call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Tools []ToolSpec `json:"tools"`
	}
	if err := json.Unmarshal(result, &parsed); err != nil {
		return nil, err
	}
	return parsed.Tools, nil
}

func (c *Client) CallTool(ctx context.Context, name string, arguments json.RawMessage) (CallResult, error) {
	if len(arguments) == 0 {
		arguments = json.RawMessage("{}")
	}
	result, err := c.call(ctx, "tools/call", map[string]any{"name": name, "arguments": arguments})
	if err != nil {
		return CallResult{}, err
	}
	var parsed struct {
		Content []Content `json:"content"`
		IsError bool      `json:"isError"`
	}
	if err := json.Unmarshal(result, &parsed); err != nil {
		return CallResult{}, err
	}
	return CallResult{Content: parsed.Content, IsError: parsed.IsError}, nil
}

func rawJSON(value any) json.RawMessage {
	if value == nil {
		return nil
	}
	data, _ := json.Marshal(value)
	return data
}
