package provider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func (standardProvider *Standard) responsesWebSocketAddress() (string, error) {
	address := standardProvider.providerSpec.WebSocketURL
	if address == "" {
		address = strings.TrimRight(standardProvider.providerSpec.APIURL, "/") + "/responses"
	}
	parsed, operationError := url.Parse(address)
	if operationError != nil {
		return "", operationError
	}
	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	case "http":
		parsed.Scheme = "ws"
	case "ws", "wss":
	default:
		return "", fmt.Errorf("Responses WebSocket URL has unsupported scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", fmt.Errorf("Responses WebSocket URL must have a host and no user information or fragment")
	}
	return parsed.String(), nil
}

func (standardProvider *Standard) responsesWebSocketHeaders(operationContext context.Context, address string) (http.Header, error) {
	request, operationError := http.NewRequestWithContext(operationContext, http.MethodGet, address, nil)
	if operationError != nil {
		return nil, operationError
	}
	request.Header = standardProvider.websocketHeaders.Clone()
	if request.Header == nil {
		request.Header = make(http.Header)
	}
	if session, found := harness.SessionFrom(operationContext); found && session.ID != "" {
		request.Header.Set("session_id", string(session.ID))
	}
	if operationError := standardProvider.applyAuthenticationHeaders(request); operationError != nil {
		return nil, operationError
	}
	return request.Header, nil
}

func (standardProvider *Standard) streamResponsesWebSocket(operationContext context.Context, payload map[string]any) (harness.Stream, error) {
	address, operationError := standardProvider.responsesWebSocketAddress()
	if operationError != nil {
		return nil, operationError
	}
	scope := responsesConnectionScope{}
	if session, found := harness.SessionFrom(operationContext); found {
		scope.instanceID = session.InstanceID
		scope.sessionID = string(session.ID)
	}
	lease, operationError := standardProvider.websockets.acquire(operationContext, scope)
	if operationError != nil {
		return nil, operationError
	}
	headers, operationError := standardProvider.responsesWebSocketHeaders(lease.context, address)
	if operationError != nil {
		lease.release()
		return nil, operationError
	}
	encodedHeaders, operationError := json.Marshal(headers)
	if operationError != nil {
		lease.release()
		return nil, operationError
	}
	identity := sha256.Sum256(append([]byte(address+"\n"), encodedHeaders...))
	if lease.entry.identity != identity {
		lease.retireConnection()
		lease.entry.httpFallback = false
		lease.entry.identity = identity
	}
	if lease.entry.httpFallback && standardProvider.providerSpec.Transport == "auto" {
		return standardProvider.streamResponsesHTTP(lease.context, payload, lease.release)
	}
	if lease.entry.socket != nil && !lease.entry.socket.reusable() {
		lease.retireConnection()
	}
	if lease.entry.socket == nil {
		if operationError := standardProvider.connectResponsesWebSocket(lease, address, headers); operationError != nil {
			fallback := standardProvider.providerSpec.Transport == "auto" && unsupportedResponsesUpgrade(operationError) && lease.context.Err() == nil
			if fallback {
				lease.entry.httpFallback = true
			}
			if fallback {
				log.Printf("mtt: provider %s transport=http; WebSocket upgrade unsupported: %v", standardProvider.Name(), operationError)
				return standardProvider.streamResponsesHTTP(lease.context, payload, lease.release)
			}
			lease.release()
			return nil, operationError
		}
	}
	stream := &providerStream{parts: make(chan atom.ResponsePart, 64), cancel: lease.cancel, onClose: lease.release}
	go standardProvider.receiveResponsesWebSocket(lease, address, headers, payload, stream)
	return stream, nil
}

func unsupportedResponsesUpgrade(operationError error) bool {
	var transportError *responseTransportError
	if !errors.As(operationError, &transportError) {
		return false
	}
	switch transportError.status {
	case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusUpgradeRequired, http.StatusNotImplemented:
		return true
	default:
		return false
	}
}

func (standardProvider *Standard) connectResponsesWebSocket(lease *responsesWebsocketLease, address string, headers http.Header) error {
	connectContext, cancel := context.WithTimeout(lease.context, 15*time.Second)
	defer cancel()
	connection, response, operationError := websocket.Dial(connectContext, address, &websocket.DialOptions{HTTPClient: standardProvider.client, HTTPHeader: headers, CompressionMode: websocket.CompressionContextTakeover})
	if operationError != nil {
		if lease.context.Err() != nil {
			return lease.context.Err()
		}
		if response != nil {
			return responsesHTTPError(standardProvider.Name(), "WebSocket upgrade", response, 0, headers)
		}
		return fmt.Errorf("%s Responses WebSocket connection: %w", standardProvider.Name(), operationError)
	}
	lease.entry.socket = newResponsesWebsocketConnection(connection)
	lease.entry.baseline = nil
	log.Printf("mtt: provider %s transport=websocket; connected", standardProvider.Name())
	return nil
}

func websocketResponsesPayload(payload map[string]any, baseline *responsesContinuation) ([]byte, bool, error) {
	frame := make(map[string]any, len(payload)+1)
	for name, value := range payload {
		if name != "stream" && name != "background" {
			frame[name] = value
		}
	}
	frame["type"] = "response.create"
	input, incremental := baseline.incrementalInput(payload)
	if incremental {
		frame["previous_response_id"] = baseline.responseID
		frame["input"] = input
	}
	encoded, operationError := json.Marshal(frame)
	return encoded, incremental, operationError
}

func (standardProvider *Standard) receiveResponsesWebSocket(lease *responsesWebsocketLease, address string, headers http.Header, payload map[string]any, stream *providerStream) {
	defer stream.finishResponse()
	for attempt := 0; attempt < 2; attempt++ {
		encoded, incremental, operationError := websocketResponsesPayload(payload, lease.entry.baseline)
		lease.entry.baseline = nil
		if operationError != nil {
			stream.operationError = operationError
			lease.retireConnection()
			return
		}
		socket := lease.entry.socket
		stopCancellation := context.AfterFunc(lease.context, func() { socket.close() })
		if operationError := socket.connection.Write(lease.context, websocket.MessageText, encoded); operationError != nil {
			stopCancellation()
			lease.retireConnection()
			stream.operationError = fmt.Errorf("%s Responses WebSocket send; request_bytes=%d: %w", standardProvider.Name(), len(encoded), incompleteWebSocketError(operationError))
			return // Delivery is uncertain; a second generation could repeat tool calls.
		}
		log.Printf("mtt: provider %s transport=websocket; request_bytes=%d; incremental=%t", standardProvider.Name(), len(encoded), incremental)
		decoder := newResponsesDecoder(lease.context, stream, standardProvider.Name())
		accepted := false
		recoverContext := false
		for {
			data, operationError := socket.nextEvent(lease.context)
			if operationError != nil {
				stream.operationError = fmt.Errorf("%s Responses WebSocket receive before response.completed; response_id=%q%s: %w", standardProvider.Name(), boundedProviderDetail(decoder.responseID), decoder.pendingFunctionDetails(), incompleteWebSocketError(operationError))
				break
			}
			var event responseEvent
			if json.Unmarshal(data, &event) == nil {
				if !accepted && attempt == 0 && event.Type == "error" && event.Error != nil && event.Status == http.StatusBadRequest {
					code := event.Error.code()
					recoverContext = (incremental && code == "previous_response_not_found") || code == "websocket_connection_limit_reached"
					if recoverContext {
						break
					}
				}
				if strings.HasPrefix(event.Type, "response.") {
					accepted = true
				}
			}
			keepReading, decodeError := decoder.consumeEvent(data)
			if decodeError != nil {
				stream.operationError = fmt.Errorf("%s; request_bytes=%d; incremental=%t", redactProviderDetail(decodeError.Error(), headers), len(encoded), incremental)
				break
			}
			if !keepReading {
				break
			}
		}
		stopCancellation()
		if lease.context.Err() != nil {
			stream.operationError = lease.context.Err()
			lease.retireConnection()
			return
		}
		if recoverContext {
			lease.retireConnection()
			if operationError := standardProvider.connectResponsesWebSocket(lease, address, headers); operationError != nil {
				stream.operationError = operationError
				return
			}
			continue // Explicit rejection before response.created; send the full context once.
		}
		if stream.operationError != nil || decoder.completed == nil {
			lease.retireConnection()
			return
		}
		lease.entry.baseline = captureResponsesContinuation(payload, decoder.completed, standardProvider.Name())
		return
	}
}

func incompleteWebSocketError(operationError error) error {
	// harness.Stream reserves io.EOF for a successfully completed model response.
	// A peer disappearing during send/read must retain partial output as an error.
	if errors.Is(operationError, io.EOF) {
		return io.ErrUnexpectedEOF
	}
	return operationError
}
