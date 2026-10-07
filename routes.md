# Harness API Contracts

The document gives the HTTP and WebSocket API contracts. The source is `internal/api/server.go`, the handler files, and the `atom` types. Use the code as the source for an API contract check.

## Connection and Authentication

The default Docker URL is `http://localhost:18080`. The route paths start at the harness URL. The UI proxy adds the `/api` prefix. The harness does not use the prefix.

`GET /health` is open. Token authentication is necessary for the other paths. Use a header for HTTP requests:

```http
Authorization: Bearer TOKEN
Content-Type: application/json
```

The handler accepts `?token=TOKEN` when the header does not contain a `Bearer` value. A browser WebSocket client can use the query parameter. The header value applies when the request also has a query value. An authentication error gives `401`. A settings store error gives `503`.

The request body has a limit of 16 MiB. It must contain one JSON value. The decoder accepts unknown object fields. A JSON error gives `400`.

The handlers give JSON responses. An error response has the shape:

```json
{"error":"description of the failed operation"}
```

The HTTP router can give a text response for an unknown path or an incorrect method. The harness does not have a CORS handler. The UI uses a same-origin proxy.

## JSON Format

The atom response keys are `ID`, `Workspace`, `Content`, and `CreatedAt`, for example. Request DTO keys include `workspace` and `default_model`. Do not change the keys.

A timestamp uses the `RFC 3339` format. A duration in a Go data type is an integer in nanoseconds. The duration unit applies to the fields:

```text
Provider.Interval
ProcessSpec.Timeout
ProcessSpec.Notify.Interval
ToolResult.Duration
```

The `bash` tool input uses milliseconds for `timeout` and `interval`.

A byte slice uses a Base64 string or `null` in JSON. The fields `ToolCall.Input`, `Event.Payload`, and `Schema.JSON` contain JSON values.

A list can be `null`. The message, agent, process, and provider handlers give empty arrays when data is not available. Other handlers can give `null`.

## Instance Routes

```text
| Method | Path | Input | Success | Handler errors |
|---|---|---|---|---|
| GET | `/instances` | none | `200`, `Instance[]` or null | `500` |
| POST | `/instances` | `InstanceInput` | `201`, `Instance` | `400`, `500` |
| GET | `/instances/{id}` | none | `200`, `Instance` | `404` |
| DELETE | `/instances/{id}` | none | `200`, `{"status":"stopped"}` | `404`, `500` |
| POST | `/instances/{id}/start` | none | `200`, `Instance` | `404`, `400`, `409`, `500` |
| GET | `/instances/{id}/models` | none | `200`, `Model[]` or null | `404` |
| GET | `/instances/{id}/sessions` | none | `200`, `Session[]` | `404`, `500` |
| POST | `/instances/{id}/sessions` | optional `{"model":"provider/model","reasoning_effort":"high"}` | `201`, `Session` | `404`, `400`, `500` |
| GET | `/instances/{id}/statistics` | none | `200`, `Statistics` | `500` |
```

`InstanceInput`:

```json
{
  "workspace": "/workspace",
  "models": ["test/test-model"],
  "default_model": "test/test-model",
  "process_limit": 8,
  "agent_depth_limit": 2
}
```

The workspace must be a directory on the server. A relative path starts at the process directory. An empty model list lets the instance use available models. The session model must be in the instance model list. Without a model value, the session uses the instance default.

The fields `process_limit` and `agent_depth_limit` set an instance configuration value when the input is above 0. A value of 0 does not set a limit in the instance request. To set a value of 0, send a request to a settings route. The instance response fields can be 0 after the configuration data goes to settings.

When an instance stops, the database keeps the configuration and sessions. The active turns and processes stop. Messages in the queue can continue when the instance starts again. The instance must be active to give a model list or make a session. The API can read the configuration of a stopped instance.

## Session and Queue Routes

```text
| Method | Path | Input | Success | Handler errors |
|---|---|---|---|---|
| GET | `/sessions/{id}` | none | `200`, `Session` | `404` |
| DELETE | `/sessions/{id}` | none | `200`, `SessionDeletion` | `404`, `409`, `500`, `503` |
| PUT | `/sessions/{id}/reasoning` | `{"effort":"high"}` | `200`, `Session` | `404`, `400`, `409`, `500` |
| PUT | `/sessions/{id}/model` | `{"model":"provider/model","allow_compaction":false}` | `200`, `Session` | `404`, `400`, `409`, `500` |
| GET | `/sessions/{id}/messages` | none | `200`, `Message[]` | `500` |
| POST | `/sessions/{id}/messages` | `{"content":"text"}` or content array | `202`, `AcceptedMessage` | `404`, `400`, `409`, `429`, `500` |
| GET | `/sessions/{id}/status` | none | `200`, `QueueStatus` | `404`, `500` |
| GET | `/sessions/{id}/task-state` | none | `200`, `TaskState` | `404`, `500` |
| POST | `/sessions/{id}/cancel` | none | `200`, `{"status":"cancelled"}` | `409`, `500` |
| DELETE | `/sessions/{id}/queue/{message_id}` | none | `200`, `{"status":"removed"}` | `404`, `500` |
| POST | `/sessions/{id}/revert` | `{"message_id":"ID"}` | `200`, `RevertResult` | `404`, `400`, `500` |
| GET | `/sessions/{id}/agents` | none | `200`, session ID array | `500` |
| GET | `/sessions/{id}/statistics` | none | `200`, `Statistics` | `500` |
| GET | `/sessions/{id}/processes` | none | `200`, `ProcessRecord[]` | `500` |
| GET | `/sessions/{id}/events` | WebSocket upgrade; optional `since` | event frames | `404`, `400`, upgrade error |
```

The database keeps a message in the queue before the API gives `202`. The message goes into history when the turn starts. A session can have 128 messages that wait. The harness limit is 4096 messages that wait or run.

The session input accepts an optional `reasoning_effort` string with `model`. An empty string uses the model default. The model gives the available values in `ReasoningEfforts`. A session change applies to the next model request. It does not stop an active request. The database keeps the selection.

A model change must use the instance model list. The new model must have a context limit. A smaller context limit gives `409` with `code: context_compaction_required`. An unknown previous context limit also gives `409`. After the user accepts context compaction, send `allow_compaction: true` to change the model.

The response gives `current_context_max`, `target_context_max`, and `model` with the error. When the context is too large, the harness removes the initial turn from the request. The database keeps the full message history.

If the selection changed before the request, the API gives `409` with `code: session_selection_changed`. Then read the session again.

If the new model does not have the previous reasoning effort, `ReasoningEffort` becomes an empty string for the model default.

Client input has the role `user`. Background process notifications have the role `runtime`. They use the same queue. The tool with the name `bash` gives output through the tool result group when `wait` is `true`. It does not add a process notification to the queue.

A full queue gives `429`. A stopped instance or a completed child agent gives `409`. A queue that is closed or a revert operation can also give `409`.

```json
{
  "status": "queued",
  "position": 1,
  "message": {
    "ID": "message-id",
    "SessionID": "session-id",
    "Seq": 0,
    "Role": "user",
    "Content": [{"Type":"text","Text":"Inspect this project","Data":null,"MIME":"","URL":"","Filename":"","AudioID":""}],
    "ToolCalls": null,
    "ToolCallID": "",
    "Usage": null,
    "CreatedAt": "2026-09-28T12:00:00Z"
  }
}
```

Queue position is a snapshot when the API accepts the message. The turn can start before the client gets the response. Status has the shape:

```json
{"running":true,"queued":1,"messages":["waiting-message-id"],"error":""}
```

The `messages` field can be `null`. It contains the IDs of messages that wait. Message text and the active message ID are not included. The `running` value includes the claim operation before the provider starts. The `error` field gives a coordinator error when available.

The route `POST /sessions/{id}/cancel` tells the worker to cancel the turn. The response does not wait until the worker stops. A session without an active turn gives `409`. The route `DELETE /sessions/{id}/queue/{message_id}` removes a message that waits. It gives `404` when the message does not wait in the queue.

When the user cancels a turn, file edits stay. The background processes do not automatically stop.

The revert operation keeps the selected message and removes the messages after it. The API stops new input. It waits until the active turn stops before it removes history. The API also removes the messages from the queue.

Usage records stay because the model usage occurred. A revert operation changes conversation history, not file content.

```json
{"status":"reverted","removed":3}
```

The `removed` value counts history messages only. A coordinator error during a revert operation gives `500`. A collection route or a status route can give an empty result for an unknown session.

The route `DELETE /sessions/{id}` removes a conversation and the child sessions. The session tree must not have an active turn, messages in the queue, or a process with status `running`. A parent session must stop before the operation removes a child session. The API gives `409` if work must stop first.

The operation removes messages, events, process records, and session permission decisions. It keeps usage records. The operation also keeps workspace permission decisions. Workspace files do not change.

A tombstone keeps the session ID and parent session ID for usage statistics. A request cannot use a tombstone ID to put the conversation back into the store. The API does not show sessions with tombstones in session lists.

The queue stops new input to the selected session tree during session deletion. Other conversations can continue. The queue can complete the operation after the caller cancels the request. A storage error keeps the conversation. The queue can accept input again after a storage error.

```json
{"status":"deleted","session_ids":["parent-session-id","child-session-id"]}
```

The field `session_ids` gives the IDs of the sessions that the operation removed. A session that is not available gives `404`. A server without a session queue gives `503`.

Content request example:

```json
{"content":[{"Type":"text","Text":"Describe this image"},{"Type":"image","MIME":"image/png","Data":"BASE64_DATA"}]}
```

Content types are `text`, `image`, `audio`, and `file`. A media item must have data, a URL, or an audio ID. The loop examines model compatibility before the model call. Thus, an accepted message can give a turn error.

## Providers and Keys

```text
| Method | Path | Input | Success | Handler errors |
|---|---|---|---|---|
| GET | `/providers` | none | `200`, `Provider[]` | `500`, `503` |
| GET | `/providers/{id}/models` | none | `200`, `Model[]` | `404`, `503` |
| POST | `/providers/{id}/refresh` | none | `200`, `Model[]` | `502`, `500` |
| PUT | `/providers/{id}/key` | `{"key":"SECRET"}` | `200`, `{"status":"saved"}` | `400`, `500` |
| DELETE | `/providers/{id}/key` | none | `200`, `{"status":"deleted"}` | `500` |
| PUT | `/instances/{id}/providers/{provider}/key` | `{"key":"SECRET"}` | `200`, `{"status":"saved"}` | `400`, `500` |
| DELETE | `/instances/{id}/providers/{provider}/key` | none | `200`, `{"status":"deleted"}` | `500` |
| POST | `/providers/{id}/auth/device` | none | `202`, `DeviceLogin` | `400`, `404`, `502`, `503` |
| GET | `/providers/{id}/auth/device/{login_id}` | none | `200`, `DeviceLogin` | `400`, `404`, `503` |
| DELETE | `/providers/{id}/auth/device/{login_id}` | none | `200`, `{"status":"cancelled"}` | `400`, `404`, `503` |
| DELETE | `/providers/{id}/auth` | none | `200`, `{"status":"disconnected"}` | `400`, `404`, `500`, `503` |
```

The provider ID is the provider name. The default data for `openai`, `deepseek`, and `openai-codex` is in `providers.json`. Change the file to change or add providers. A missing or empty file does not add providers.

The API cannot add a provider. A provider route gives model IDs without the provider prefix.

An instance model list uses `provider/model`. An ID without a provider prefix is correct only when one provider has the model ID.

The instance key overrides the harness key. Provider routes do not give secret values. A key handler does not examine if the provider or instance is in the database. A harness key must not be empty. The `DELETE` method removes a harness key. An instance key can be empty.

The API key route does not accept a credential for `openai-codex`. The provider uses device authentication. The refresh route uses the harness provider configuration. For `openai-codex`, the refresh operation gets the model list from the URL in the JSON file. The request uses account authentication. The response format is `codex`.

A correct model list response replaces available models. An empty list removes available models. A request error or an incorrect response keeps the last correct list.

The field `ToolSupportUnknown` is `true` when the response does not give data about tools. Then the harness sends tool definitions. A value of `false` for `tools` in model data removes tool definitions.

The provider list contains providers in the registry and the test provider when available. A previous database provider without a registry entry is not in the response. Provider data includes the fields:

```typescript
Protocol: '' | 'responses' | 'chat_completions';
Authentication: '' | 'api_key' | 'chatgpt' | 'none';
Connected: boolean; // stored credential, not a guarantee of model access
ModelCount: number; // cached model count
```

The field `Connected` is `true` for a provider without authentication. For `openai` and `deepseek`, it shows that a harness key is in the database. For `openai-codex`, it shows that OAuth tokens are in the database. The key handler does not examine if the key gives model access. A client can send a request to the refresh route after it sets a key.

Device authentication uses the response:

```typescript
type DeviceLogin = {
  ID: string; // harness login ID, not the upstream device secret
  Provider: string; // configured provider ID
  VerificationURL: string;
  UserCode: string;
  ExpiresAt: string; // RFC 3339; device code lasts 15 minutes
  Status: 'pending' | 'connected' | 'cancelled' | 'expired' | 'error';
  Error: string;
};
```

The user opens the OpenAI URL and supplies the code. The server waits for OpenAI authorization and stores the credential. The UI can read login status at an interval of 2 seconds. A new login replaces the previous login.

Device routes use the provider ID from configuration. The authentication method must be `chatgpt`. A login is for one provider. After login, the client can refresh the model list.

The user can cancel a login that is in progress. This does not remove a credential from the database. The route `DELETE /providers/openai-codex/auth` removes the credential.

The authentication routes use the same API credential as the other routes. They send `Cache-Control: no-store`. Access tokens, refresh tokens, and the upstream device secret are not in the response. A login in progress stops when the harness stops. A credential in the database stays when the harness starts again.

## Settings

```text
| Method | Path | Input | Success | Handler errors |
|---|---|---|---|---|
| GET | `/settings` | none | `200`, string map | `500` |
| PUT | `/settings/{key}` | `{"value":"STRING"}` | `200`, `{"status":"saved"}` | `400`, `500` |
| DELETE | `/settings/{key}` | none | `200`, `{"status":"deleted"}` | `400`, `500` |
| GET | `/instances/{id}/settings` | none | `200`, string map | `500` |
| PUT | `/instances/{id}/settings/{key}` | `{"value":"STRING"}` | `200`, `{"status":"saved"}` | `400`, `500` |
| DELETE | `/instances/{id}/settings/{key}` | none | `200`, `{"status":"deleted"}` | `400`, `500` |
```

Configuration values use strings. A limit value also uses a string:

```json
{"agent_depth_limit":"2","process_limit":"8"}
```

The limit value must be 0 or more. The `api_token` value must not be empty. The token is a harness configuration value only. An instance cannot override the token, and the API cannot remove it. A token change applies to subsequent requests.

The `GET` routes give the database values for one scope. Instance values and harness values stay in different responses. A harness settings response can include `api_token`. The handler does not examine if the instance is in the database.

## Plugin Control

An API credential is necessary for plugin routes. An instance route examines the workspace in the database before the plugin operation.

```text
| Method | Path | Input | Success | Handler errors |
|---|---|---|---|---|
| GET | /plugins | none | 200, PluginState array | 500 |
| GET | /plugins/{plugin}/settings | none | 200, PluginState | 400, 404, 500 |
| PATCH | /plugins/{plugin}/settings | partial JSON object | 200, PluginState | 400, 404 |
| GET | /instances/{id}/plugins | none | 200, PluginState array | 404, 500 |
| GET | /instances/{id}/plugins/{plugin}/settings | none | 200, PluginState | 400, 404, 500 |
| PATCH | /instances/{id}/plugins/{plugin}/settings | partial JSON object | 200, PluginState | 400, 404 |
| GET | /instances/{id}/plugins/{plugin}/statistics | none | 200, PluginMetrics | 400, 404, 500 |
| GET | /instances/{id}/agent-statistics | none | 200, AgentStatistics array | 404, 500 |
```

The default plugin state for `context` is `OFF`. An available worker model is necessary for `ON`. For example:

```json
{"enabled":true,"worker_model":"provider/model"}
```

The patch changes only supplied settings. A workspace value can override a harness value. A JSON `null` removes the workspace value. The `Schema.JSON` field gives parameter descriptions, defaults, and limits.

```text
PluginState {
  Name: string, Version: string, WorkspaceID: string,
  Available: boolean, Enabled: boolean, RequestedEnabled: boolean,
  Pending: boolean, Configuration: object, Override: object,
  Schema: { JSON: object }, Error?: string
}

PluginMetrics {
  Name: string, WorkspaceID: string,
  Counters: map<string, integer>, Agents: AgentStatistics[] | null
}

AgentStatistics {
  Agent: string, ModelID: string, Statistics: Statistics,
  FailedCalls: integer, DurationMilliseconds: integer
}
```

`Configuration` is the resolved configuration. `Override` contains database fields for the selected scope. An empty `WorkspaceID` identifies harness settings. A client must use `Enabled` and `Pending` to show the plugin state. The value of `RequestedEnabled` does not identify when the harness completes the operation.

Memory jobs stay in the database at `OFF`. The runtime stops new work after accepted tool groups and cancels active workers. The memory archive and the context view given to the model stay available. Workspace agent cost does not increase session cost.

See `plugins/context/README.md` for configuration and memory behavior.

## Permission and Process Routes

```text
| Method | Path | Input | Success | Handler errors |
|---|---|---|---|---|
| POST | `/permissions/{id}` | `PermissionInput` | `200`, `{"status":"resolved"}` | `400`, `404` |
| GET | `/statistics` | none | `200`, `Statistics` | `500` |
| GET | `/health` | none | `200`, `{"status":"ok"}` | none |
| GET | `/processes/{id}/output` | WebSocket upgrade | output frames | `404`, upgrade error |
```

```json
{"kind":"allow","scope":"once"}
```

Permission type is `allow` or `deny`. Scope is `once`, `session`, or `always`. An empty scope or a request without a scope uses `once`. A decision in the database applies in the instance of the decision. A request that is not open gives `404`.

The API does not have a route to read open permission requests. Session events give the requests and decisions.

Session statistics include the usage of child agents. The cost list has an entry for a currency. A `null` cost value does not show free usage. It shows that cost data is not available.

The `Input` value does not include `CacheRead` or `CacheWrite`. The cache ratio is `CacheRead / (Input + CacheRead + CacheWrite)`. The cache ratio is 0 when the total input is 0.

The `/health` route gives a liveness response. It does not examine the provider or database. The output route uses a harness process ID, not an OS PID.

The harness keeps a maximum of 256 KiB for a process stream. It keeps output from the last 128 completed processes. The database can keep process data after the output is removed. The API does not have an HTTP route to stop a process.

## WebSocket Data

Session event URL:

```text
ws://localhost:18080/sessions/SESSION_ID/events?token=TOKEN&since=0
```

The `since` value is a sequence cursor. The default is 0. The range is `0..9223372036854775807`. The server gives the database events after the cursor, then new session events.

The database sequence includes events from different sessions. Session sequence values can have a difference above 1. Use the last received sequence number when the client connects again.

```json
{"Seq":42,"InstanceID":"instance-id","SessionID":"session-id","Name":"model.chunk","Payload":{"text":"Hello"},"Time":"2026-09-28T12:00:00Z"}
```

The server sends JSON text frames. The client receives events on the route. It must not send commands on the socket. The client must open a new connection after a socket error. The server closes a connection if output cannot complete in 5 seconds.

Event payload data:

```text
| Event | Payload |
|---|---|
| `turn.start` | null |
| `turn.end` | `{"status":"completed"}`, `cancelled`, or `error` |
| `model.call` | `{"model":"provider/model","message_id":"ID","reasoning_effort":"high"}` |
| `model.chunk` | `{"message_id":"ID","text":"fragment"}` or `{"message_id":"ID","reasoning":"text"}` |
| `action.received` | `ToolCall` |
| `tool.start` | `ToolCall` |
| `tool.end` | `{"call":ToolCall,"status":"ok","result":ToolResult}` |
| `task_state.updated` | `TaskState` |
| `permission.request` | `PermissionRequest` |
| `permission.decision` | `PermissionDecision` |
| `run.error` | `{"message_id":"ID","error":"description"}` |
| `run.cancelled` | `{"message_id":"ID","error":"description"}` |
| `run.interrupted` | `{"message_id":"ID"}` |
| `process.start`, `process.output`, `process.exit` | `ProcessEvent` |
| `process.notification` | null |
| `agent.start` | `{"session":"child-session-id","task":"task text"}` |
| `agent.end` | `{"session":"child-session-id"}` |
```

A plugin can use other event IDs and payloads. A constant in `atom/event.go` does not show that a handler sends the event.

The field `message_id` identifies the model message in the history. The client uses the ID to keep one message in the display. Reasoning events contain only reasoning text or reasoning summaries from the provider. The API does not send continuation data.

Process output URL and frame:

```text
ws://localhost:18080/processes/PROCESS_ID/output?token=TOKEN
```

```json
{"stream":"stdout","data":"command output\n","error":""}
```

The process stream does not have a sequence cursor. It gives the output buffer, then new output. Stream values are `start`, `stdout`, `stderr`, and `exit`. The socket closes at exit.

The `data` field is a string in the process stream. The `ProcessEvent.Data` field uses Base64 in a session event.

## Data Types

### Task State

The task state route gives the same keys as `atom.TaskState`:

```json
{
  "SessionID":"session-id",
  "Todo":[
    {"ID":"1","Title":"Examine the parser","Status":"done"},
    {"ID":"2","Title":"Verify the correction","Status":"in_progress"}
  ],
  "Doing":{"Title":"Verifying the correction","Description":"Running the parser test suite."},
  "Revision":2,
  "UpdatedAt":"2026-10-06T12:00:00Z"
}
```

Status values are `pending`, `in_progress`, `done`, and `cancelled`. Before the initial task update, a session gives `Todo: []`, `Doing: null`, and revision 0. The `UpdatedAt` value is `0001-01-01T00:00:00Z` before a task update. The revision does not decrease after a task update. The API does not give the response counter.

The model uses `task_state` to change task state. The client reads the route and `task_state.updated` events. A previous revision must not replace a new revision. The client must not apply task state from a different session.

See `docs/task-state.md` for task update rules.

The code gives the JSON data types in TypeScript. It is not a harness dependency. A list can be `null` where shown. A duration uses nanoseconds.

```typescript
type Instance = {
  ID: string; Workspace: string; Models: string[] | null; DefaultModel: string;
  ProcessLimit: number; AgentDepthLimit: number; CreatedAt: string; Stopped: boolean;
};
type Session = {
  ID: string; InstanceID: string; Parent: string; Depth: number;
  Model: string; ReasoningEffort?: string; CreatedAt: string; Completed: boolean;
};
type Content = {
  Type: 'text' | 'image' | 'audio' | 'file'; Text: string;
  Data: string | null; MIME: string; URL: string; Filename: string; AudioID: string;
};
type ToolCall = { ID: string; Name: string; Input: unknown };
type ToolResult = {
  CallID: string; Status: 'ok' | 'denied' | 'error'; Content: Content[] | null;
  Error: string; Duration: number;
};
type Message = {
  ID: string; SessionID: string; Seq: number; Role: 'system' | 'user' | 'assistant' | 'tool' | 'runtime';
  Content: Content[] | null; ToolCalls: ToolCall[] | null; ToolCallID: string;
  Usage: Usage | null; Reasoning?: string; CreatedAt: string;
};
type Cost = { Currency: string; Value: number; Estimated?: boolean };
type Usage = {
  Input: number; CacheRead: number; CacheWrite: number; Output: number;
  Reasoning: number; Cost: Cost | null;
};
type Statistics = Usage & {
  Calls: number; Costs: Cost[] | null; CacheHitRate: number; CacheHitPercentage: number;
};
type Provider = {
  Name: string; APIURL: string; ModelListURL: string; PriceTableURL: string; Interval: number;
  Protocol: '' | 'responses' | 'chat_completions';
  ModelListFormat?: 'openai' | 'codex';
  Authentication: '' | 'api_key' | 'chatgpt' | 'none';
  Connected: boolean; ModelCount: number;
  MetadataURL?: string; MetadataFormat?: 'models_dev'; MetadataProvider?: string;
  Billing?: 'tokens' | 'subscription';
};
type Model = {
  ID: string; Name?: string; Level: number; Input: string[] | null; Output: string[] | null;
  Tools: boolean; ToolSupportUnknown?: boolean; ContextMax: number;
  Reasoning?: boolean; ReasoningEfforts?: string[]; DefaultReasoningEffort?: string;
  ReasoningSummary?: string; Billing?: 'tokens' | 'subscription';
  Prices: null | (PriceRates & { Currency: string; Source?: string; Tiers?: PriceTier[] });
};
type PriceRates = {
  Input: number; Output: number; CacheRead: number; CacheWrite: number; Reasoning?: number;
  CacheReadUnknown?: boolean; CacheWriteUnknown?: boolean;
};
type PriceTier = PriceRates & { AboveInputTokens: number };
type PermissionRequest = { ID: string; InstanceID: string; SessionID: string; Target: string; Why: string };
type PermissionDecision = { RequestID: string; InstanceID: string; SessionID: string; Target: string; Kind: 'allow' | 'deny'; Scope: 'once' | 'session' | 'always'; CreatedAt: string };
type ProcessRecord = {
  ID: string; InstanceID: string; SessionID: string; PID: number; Status: string;
  Spec: { Command: string; Args: string[] | null; Cwd: string; Env: string[] | null;
    Notify: { Mode: 'exit' | 'error' | 'interval'; Interval: number }; Timeout: number };
  Exit: null | { Code: number; Signal: string; Error: string }; StartedAt: string; EndedAt: string;
};
type ProcessEvent = { ProcessID: string; Stream: 'start' | 'stdout' | 'stderr' | 'exit'; Data: string | null; Error: string; At: string };
```

Model prices apply to 1000000 tokens. The `ContextMax` field is the model token limit. The `Input` and `Output` lists give the media types of the model. The `Tools` field shows if the model can use tools.

The optional field `Name` is a display label. Model requests use `ID`. For example, `deepseek-flash` is the API ID for `DeepSeek-V4.1-Flash`. The ID `deepseek-v4-pro` refers to `DeepSeek-V4-Pro-0813`. See the [DeepSeek model data](https://api-docs.deepseek.com/quick_start/pricing) for the provider data.

The harness calculates cost from provider usage. It does not calculate cost from the context limit estimate.

The field `Estimated` has the value `true` for a cost estimate or a total with a cost estimate. A price tier uses the full input tokens with cache tokens. Reasoning tokens are part of output tokens. The harness does not calculate token cost estimates for subscription models. A price of 0 is different from price data that is not available.
