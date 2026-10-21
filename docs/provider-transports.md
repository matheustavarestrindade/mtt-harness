# Provider Connections

## Configuration

The `protocol` field selects the provider message format. The `transport` field selects communication for the Responses protocol. Provider configuration is in `providers.json`.

- `http`: Send a full HTTP request. Read response events from SSE. This is the default value.
- `websocket`: Use a WebSocket connection. A handshake error is a request error.
- `auto`: Use a WebSocket connection. HTTP can be used after status 404, 405, 426, or 501.

WebSocket transport is available for `protocol: responses`. The OpenAI and Codex configurations use `auto`. Other providers keep the selected transport.

The adapter makes the WebSocket URL from `api_url` and `/responses`. It uses `ws` for HTTP and `wss` for HTTPS. The optional `websocket_url` field replaces that URL. Handshake headers are in the optional `websocket_headers` object. Provider API responses do not contain the handshake headers.

```json
{
  "name": "compatible-provider",
  "protocol": "responses",
  "authentication": "api_key",
  "api_url": "https://api.example/v1",
  "transport": "auto",
  "websocket_url": "wss://api.example/v1/responses"
}
```

Credentials use the database secret store. The `websocket_headers` object cannot set authentication headers. The Codex configuration includes `OpenAI-Beta: responses_websockets=2026-02-06`. OpenAI API access and ChatGPT subscription access use the same transport code.

## Request and Response

The client sends a text frame with `type: response.create`. The payload uses the Responses request fields, without `stream` or `background`. Model selection, reasoning effort, service tier, content, and tool schemas use one encoder.

The HTTP and WebSocket paths use one event decoder. Text, reasoning summaries, tool calls, usage, and provider continuation data use the same rules. A tool call can start before the response completes. The event `response.completed` is the necessary last event.

A connection EOF before `response.completed` is an error. The harness keeps available text and usage. An HTTP 503 response does not identify a cause. Provider error text can supply more information. The request ID can also be available.

## Tool Call Events

The adapter collects tool call data from the subsequent events:

```text
response.output_item.added
response.function_call_arguments.delta
response.function_call_arguments.done
response.output_item.done
```

The adapter uses `item_id`, `call_id`, and `output_index`. It does not make a new ID, tool name, or tool input value. The adapter can complete a tool call with supplied event data. A tool call is sent one time.

The `status` can be `incomplete`. The adapter does not run a tool for `incomplete`. It waits for the last provider event. The event can give the cause and usage.

```text
Terminal events:
response.completed
response.incomplete
response.failed
```

The `call_id`, `name`, and `arguments` values must stay the same after a tool call is sent. For `previous_response_id`, the last response must have the tool call data.

## Continuation

The connection can keep the last response ID and input fingerprints. The next request can use `previous_response_id` with only new input. The previous input, response output, and request parameters must be the same. The adapter compares tool argument JSON as data.

JSON whitespace can change without a data change. The JSON key sequence can also change. Number precision and string values stay the same.

A change to history, a system message, tools, model, reasoning effort, or request parameters can stop continuation. The adapter opens a new connection after a credential or header change. It then sends the full selected input. Full requests keep the necessary reasoning data.

The connection data is not in API responses or session history. After the harness starts, a connection starts without a previous response ID. Context measurement uses the full logical request. Billing uses provider usage and cache data. Transport bytes and token usage are different values.

## Connection Rules

With `auto`, HTTP can be used after handshake status 404, 405, 426, or 501. The adapter has not sent a model request before the handshake response. Other status values give an error. The adapter must not send a request again after it receives part of a response. The same rule applies when the transmission status is unknown.

The error `previous_response_not_found` can occur after a cache entry is removed. Before `response.created`, the adapter can open 1 new connection and send 1 full request. The error `websocket_connection_limit_reached` uses the same rule. If the new request has an error, the adapter gives the error. A new connection starts without a previous response ID.

Cancellation closes the local socket. The provider can continue work after the connection closes. The adapter stops the reader before it releases the connection gate. The next request uses a new connection and full selected input.

## Limits

- A provider keeps a maximum of 64 connections. The limit includes worker connections.
- A workspace and session have a different connection. A connection has a maximum of 1 active response.
- A worker without a session ID uses a temporary connection for 1 request.
- The idle connection limit is 5 minutes. A worker examines the connection pool. The worker period is 60 seconds.
- The connection age limit is 55 minutes. A new request uses a new connection after the limit.
- The handshake time limit is 15 seconds. Model requests use the request cancellation context.
- A response event has an 8 MiB limit. The reader can keep 1 event before the decoder.
- A response can have a maximum of 1024 tool calls. The total tool input limit is 8 MiB.
- Input fingerprints have a maximum of 16384 request items. A larger request uses full input.

The harness stops model workers, connection readers, and catalog workers before it closes storage. Catalog workers stop before the harness closes the credential store.

## Error Information

The provider log identifies the transport. It also gives the request byte count and identifies continuation. The provider log does not contain request text or credentials.

HTTP error data can include the request ID and `Retry-After` value. Provider error text has a limit of 2048 bytes. The adapter removes credential values from the text.

Error data can include the subsequent fields:

```text
response_id, item_id, call_id, output_index
tool, item_status, arguments_bytes, arguments_valid_json
```

Tool input text is not in error data. The provider can give `reason=max_output_tokens`. The adapter does not send the model request again after a tool call.

## Source Documents

- [OpenAI WebSocket mode](https://developers.openai.com/api/docs/guides/websocket-mode)
- [Codex client](https://github.com/openai/codex/blob/main/codex-rs/core/src/client.rs)
- [Codex WebSocket endpoint](https://github.com/openai/codex/blob/main/codex-rs/codex-api/src/endpoint/responses_websocket.rs)
- [WebSocket library](https://github.com/coder/websocket/tree/v1.8.15)
- [DeepSeek Responses stream](https://api-docs.deepseek.com/guides/responses_api)
- [OpenAI event type](https://github.com/openai/openai-python/blob/main/src/openai/types/responses/response_function_call_arguments_done_event.py)
