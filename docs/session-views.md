# Conversation Data

## Session Socket

The UI uses the session socket with `view=true`:

```text
GET /sessions/{id}/events?view=true&details=true
```

The connection starts with a state frame. The connection then gives events and new state frames. The `deleted` frame shows that the harness removed the session. The URL without `view=true` keeps the previous event format.

```text
State frame data
- Session and queue status
- Usage, task state, and open permissions
- History metadata and active session IDs
- Process metadata and a limited live response
- A separate TaskStateError when task-state data is unavailable
```

The queue and loop send a metadata signal after a session update. A socket worker reads a view snapshot after a metadata signal. Session coordinators send only metadata. The UI reads status from state frames. The WebSocket ping interval is 25 seconds.

The `covered_cursor` identifies events in the snapshot. The UI uses the live response revision and does not show the same output chunk again. The messages in the database stay the same.

```text
Live text limit             100000 UTF-8 bytes
Live reasoning limit        100000 UTF-8 bytes
Live tool-call limit        32
Live tool-input limit        256 KiB
Truncated                   true when the preview omits data
```

The `details=false` mode does not send live text or tool data. When an agent panel closes, an active agent uses `details=false`. If the agent is not active, the UI closes the session socket.

The connection does not send the full output history. New agents keep parent message and tool call IDs in the session. The API uses the sequence of events for previous agent origins. Task text and timestamps do not identify an agent origin.

## History Pages

```text
GET /sessions/{id}/history?limit=50
GET /sessions/{id}/history?before=123&limit=50&revision=2
GET /sessions/{id}/history?after=123&limit=50&revision=2

Default or zero limit       50 messages
Maximum limit               100 messages
Position                    exclusive saved message sequence
No position or zero         latest messages
Message order               ascending sequence
Page byte budget            2 MiB
Oversized message           one intact message is allowed
```

The query cannot use `before` and `after` together. The API does not remove bytes from a larger message. The `Context` field connects a tool result to the tool owner. The field contains the necessary tool calls. The full tool owner is available on a previous history page.

```text
HistoryPage fields
SessionID, Revision, Messages, Context, Models, Sizes,
FirstSeq, LastSeq, HeadSeq, HasOlder, HasNewer, LatestResponse,
Processes, Task
```

The `metadata=true` mode has an empty body window. Head position and last response usage stay available. Usage does not change to a previous response when the user reads a previous history page. `ProviderState` is not in a history page.

The `revert` operation increases `Revision`. A request with a different revision gives `409`. The database keeps history and model-selection revisions during session updates. Session deletion gives `404`.

## Browser Data

The UI starts with 50 messages. Use the scrollbar to read history pages. A DOM anchor keeps the reading position.

```text
History window limit        150 messages
History data budget         8 MiB, with one intact oversized message
At the latest output        50 messages
Additional data             sparse tool owner and limited live preview
```

The UI removes previous views with `URL.revokeObjectURL`, `Terminal.dispose`, and `WebSocket.close`. The message data is released. Event data, process metadata, model maps, and queue previews also have retention limits.

The UI reads status from the session socket. After a socket error, `GET /sessions/{id}/state` can give the status data. The HTTP request interval is 5 seconds. A new state frame stops the HTTP timer.
