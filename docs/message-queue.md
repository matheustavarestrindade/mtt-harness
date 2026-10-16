# Message Queue

The API keeps a message before it gives `202`. The user can send messages while a model response or a tool is active.

## Model Requests

Before the next model request, the session coordinator selects the messages that wait. They are a message batch. The message batch keeps the queue sequence. The message IDs, content, and roles stay the same.

The active response and the full tool result group finish before the next step. A worker puts the message batch into history as one transaction. If the transaction has an error, the message batch stays in the queue. The transaction does not add part of the message batch to history.

Example:

```text
Request 1: initial user message
During request 1: messages A, B, and C enter the queue
Request 2: previous history + complete tool results + A + B + C
```

The loop can continue after a text response when new input is available. The model request includes `A`, `B`, and `C`. A message that the API receives after the message batch starts is input for the next model request.

The request must include new input from the message batch. If new input is not in the request, the harness gives an error. The messages stay in history.

## Cancellation

`DELETE /sessions/{id}/queue/{message_id}` removes a message before the message batch starts. The route gives `404` if the message is not in the queue. `POST /sessions/{id}/cancel` cancels the active turn.

Cancellation waits for a database transaction that has started. The worker keeps the message IDs after the transaction completes. The worker removes the IDs from the queue.

The next program start reads the queue. A message which did not start can continue. The harness writes `run.interrupted` for a turn which cannot continue after the program stops. The harness does not run the tool plan of the turn again.

Runtime notifications keep the `runtime` role. They do not become user messages.

## UI

The message has a `Queued` label and an `X` control. The UI does not show a queue panel. The UI removes the label and control when history includes the message. An error from cancellation keeps the message in the view.

The UI can show a message ID without content when a different client sent it. The UI does not add content to the message.
