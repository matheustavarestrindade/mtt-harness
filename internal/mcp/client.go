package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
	Send(operationContext context.Context, data []byte) error
	Receive(operationContext context.Context) ([]byte, error)
	Close() error
}

type Client struct {
	name      string
	transport Transport
	timeout   time.Duration
	mutex     sync.Mutex
	next      int64
	pending   map[int64]chan rpcMessage
	closed    bool
	onChanged func(operationContext context.Context)
	changes   chan struct{}
	lifecycle context.Context
	cancel    context.CancelFunc
}

func Start(operationContext context.Context, serverSpec ServerSpec, timeout time.Duration) (*Client, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	var transport Transport
	var operationError error
	switch {
	case serverSpec.Command != "":
		transport, operationError = startStdio(serverSpec)
	case serverSpec.URL != "":
		transport, operationError = startHTTP(serverSpec)
	default:
		return nil, errors.New("mcp: the server data has no command and no URL")
	}
	if operationError != nil {
		return nil, operationError
	}
	lifecycle, stop := context.WithCancel(context.Background())
	client := &Client{
		name:      serverSpec.Name,
		transport: transport,
		timeout:   timeout,
		pending:   map[int64]chan rpcMessage{},
		changes:   make(chan struct{}, 1), lifecycle: lifecycle, cancel: stop,
	}
	go client.read()
	go client.dispatchChanges()
	initContext, cancel := context.WithTimeout(operationContext, timeout)
	defer cancel()
	initialization, operationError := client.call(initContext, "initialize", map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "mtt-harness", "version": "0.1.0"},
	})
	if operationError != nil {
		_ = client.Close()
		return nil, fmt.Errorf("mcp: the initialize of the server %s: %w", serverSpec.Name, operationError)
	}
	var initialized struct {
		Version      string `json:"protocolVersion"`
		Capabilities struct {
			Tools struct {
				ListChanged bool `json:"listChanged"`
			} `json:"tools"`
		} `json:"capabilities"`
	}
	if operationError := json.Unmarshal(initialization, &initialized); operationError != nil {
		client.Close()
		return nil, operationError
	}
	if initialized.Version != ProtocolVersion && initialized.Version != "2025-03-26" && initialized.Version != "2024-11-05" {
		client.Close()
		return nil, fmt.Errorf("MCP server selected unsupported protocol %q", initialized.Version)
	}
	if remote, supported := transport.(*httpTransport); supported {
		remote.SetProtocolVersion(initialized.Version)
	}
	if operationError := client.notify(operationContext, "notifications/initialized", nil); operationError != nil {
		_ = client.Close()
		return nil, operationError
	}
	if remote, supported := transport.(*httpTransport); supported && initialized.Capabilities.Tools.ListChanged {
		remote.StartNotifications()
	}
	return client, nil
}

func (client *Client) Name() string {
	return client.name
}

func (client *Client) OnToolsChanged(handler func(operationContext context.Context)) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	client.onChanged = handler
}

func (client *Client) call(operationContext context.Context, method string, params any) (json.RawMessage, error) {
	operationContext, cancel := context.WithTimeout(operationContext, client.timeout)
	defer cancel()
	stop := context.AfterFunc(client.lifecycle, cancel)
	defer stop()
	parameters, operationError := encodeParameters(params)
	if operationError != nil {
		return nil, operationError
	}
	client.mutex.Lock()
	if client.closed {
		client.mutex.Unlock()
		return nil, errors.New("mcp: the connection is closed")
	}
	client.next++
	identifier := client.next
	channel := make(chan rpcMessage, 1)
	client.pending[identifier] = channel
	client.mutex.Unlock()
	data, operationError := json.Marshal(rpcMessage{JSONRPC: "2.0", ID: &identifier, Method: method, Params: parameters})
	if operationError != nil {
		client.discard(identifier)
		return nil, operationError
	}
	if operationError := client.transport.Send(operationContext, data); operationError != nil {
		client.discard(identifier)
		return nil, operationError
	}
	select {
	case message, found := <-channel:
		if !found {
			return nil, errors.New("mcp: the connection is closed")
		}
		if message.Error != nil {
			return nil, fmt.Errorf("mcp: %s", message.Error.Message)
		}
		return message.Result, nil
	case <-operationContext.Done():
		client.discard(identifier)
		return nil, operationContext.Err()
	}
}

func (client *Client) notify(operationContext context.Context, method string, params any) error {
	operationContext, cancel := context.WithTimeout(operationContext, client.timeout)
	defer cancel()
	parameters, operationError := encodeParameters(params)
	if operationError != nil {
		return operationError
	}
	data, operationError := json.Marshal(rpcMessage{JSONRPC: "2.0", Method: method, Params: parameters})
	if operationError != nil {
		return operationError
	}
	return client.transport.Send(operationContext, data)
}

func (client *Client) discard(identifier int64) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	delete(client.pending, identifier)
}

func (client *Client) read() {
	for {
		data, operationError := client.transport.Receive(client.lifecycle)
		if operationError != nil {
			client.fail()
			return
		}
		var message rpcMessage
		if operationError := json.Unmarshal(data, &message); operationError != nil {
			continue
		}
		if message.ID == nil {
			if message.Method == "notifications/tools/list_changed" {
				select {
				case client.changes <- struct{}{}:
				default:
				}
			}
			continue
		}
		if message.Method != "" {
			go client.answerRequest(message)
			continue
		}
		client.mutex.Lock()
		channel, found := client.pending[*message.ID]
		if found {
			delete(client.pending, *message.ID)
		}
		client.mutex.Unlock()
		if found {
			channel <- message
		}
	}
}

func (client *Client) fail() {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	client.closed = true
	client.cancel()
	for identifier, channel := range client.pending {
		close(channel)
		delete(client.pending, identifier)
	}
}

func (client *Client) Close() error {
	client.fail()
	return client.transport.Close()
}

func (client *Client) ListTools(operationContext context.Context) ([]ToolSpec, error) {
	operationContext, cancel := context.WithTimeout(operationContext, client.timeout)
	defer cancel()
	var tools []ToolSpec
	cursor := ""
	seen := map[string]bool{}
	for {
		parameters := map[string]any{}
		if cursor != "" {
			parameters["cursor"] = cursor
		}
		result, operationError := client.call(operationContext, "tools/list", parameters)
		if operationError != nil {
			return nil, operationError
		}
		var page struct {
			Tools      []ToolSpec `json:"tools"`
			NextCursor string     `json:"nextCursor"`
		}
		if operationError := json.Unmarshal(result, &page); operationError != nil {
			return nil, operationError
		}
		tools = append(tools, page.Tools...)
		if len(tools) > 10000 {
			return nil, fmt.Errorf("MCP tool catalog exceeds 10000 tools")
		}
		if page.NextCursor == "" {
			return tools, nil
		}
		if seen[page.NextCursor] {
			return nil, fmt.Errorf("MCP pagination repeats a cursor")
		}
		cursor = page.NextCursor
		seen[cursor] = true
	}
}

func (client *Client) CallTool(operationContext context.Context, name string, arguments json.RawMessage) (CallResult, error) {
	if len(arguments) == 0 {
		arguments = json.RawMessage("{}")
	}
	result, operationError := client.call(operationContext, "tools/call", map[string]any{"name": name, "arguments": arguments})
	if operationError != nil {
		return CallResult{}, operationError
	}
	var parsed struct {
		Content []Content `json:"content"`
		IsError bool      `json:"isError"`
	}
	if operationError := json.Unmarshal(result, &parsed); operationError != nil {
		return CallResult{}, operationError
	}
	return CallResult{Content: parsed.Content, IsError: parsed.IsError}, nil
}

func (client *Client) ReportError(operationError error) {
	log.Printf("mcp[%s]: %v", client.name, operationError)
}

func encodeParameters(parameters any) (json.RawMessage, error) {
	if parameters == nil {
		return nil, nil
	}
	return json.Marshal(parameters)
}

func (client *Client) dispatchChanges() {
	for {
		select {
		case <-client.lifecycle.Done():
			return
		case <-client.changes:
			client.mutex.Lock()
			handler := client.onChanged
			client.mutex.Unlock()
			if handler != nil {
				handler(client.lifecycle)
			}
		}
	}
}

func (client *Client) answerRequest(request rpcMessage) {
	response := rpcMessage{JSONRPC: "2.0", ID: request.ID, Result: json.RawMessage(`{}`)}
	if request.Method != "ping" {
		response.Result = nil
		response.Error = &rpcError{Code: -32601, Message: "method not supported"}
	}
	data, operationError := json.Marshal(response)
	if operationError != nil {
		client.ReportError(operationError)
		return
	}
	operationContext, cancel := context.WithTimeout(client.lifecycle, client.timeout)
	defer cancel()
	if operationError := client.transport.Send(operationContext, data); operationError != nil {
		client.ReportError(operationError)
	}
}
