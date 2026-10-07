# Task State

## Purpose

Task state gives the user a TODO list and a `DOING` object for one session. After you start the harness again, the previous task state is available from Postgres. A child session has different task state from the parent session.

Task tracking is optional. The model gets the `task_state` definition through `search_tool` before a tool call.

## Task Items

A task item has:

```text
id:      1-64 ASCII letters, digits, hyphens, or underscores
title:   1-160 characters of non-blank text
status:  pending | in_progress | done | cancelled
```

The ID of a task item does not change. A task title is necessary for a new ID. The default task status is `pending`. The list can have 32 task items. A new task item is added to the end of the list.

The tool changes only the task items and fields in the input. A task update with an error does not change the data. An ID can occur only one time in a task update.

## The DOING Object

The `DOING` object has a task title and description. The task title has a limit of 120 characters. The description has a limit of 1200 characters. The 2 fields are necessary for a `DOING` object. A value of `null` removes content from `DOING`.

The description gives data about the step, goal, or problem. The model keeps it short. A TODO list is not necessary for a `DOING` object.

## Task Updates

One tool call can make the list and the `DOING` object:

```json
{
  "todo": [
    {"id":"1","title":"Examine the parser","status":"in_progress"},
    {"id":"2","title":"Verify the correction","status":"pending"}
  ],
  "doing": {
    "title":"Examining the parser",
    "description":"Reading the input path and the tests for the reported error."
  }
}
```

The full list is not necessary for a task update:

```json
{
  "todo": [
    {"id":"1","status":"done"},
    {"id":"2","status":"in_progress"}
  ],
  "doing": {
    "title":"Verifying the correction",
    "description":"Running the test suite for the parser."
  }
}
```

The database changes task state in one transaction. A field that is not in the input does not change. An empty `todo` array removes content from TODO and `DOING`. A new `DOING` object in the same tool call can start a task without a list.

The tool keeps task items with status `done` or `cancelled` while a task item has status `pending` or `in_progress`. The tool removes content from the 2 fields when the last task item becomes `done` or `cancelled`. The model can also use the input:

```json
{"todo":[],"doing":null}
```

The tool gives only `Updated` or `Cleared`. An error gives the operation and cause. The API and model context give the task state.

## Task Reminders

A task update puts the response counter at 0. The harness increases the response counter after the model completes a response with active task state. Stream chunks and tool calls do not increase it. The response counter uses the revision from the model request. It does not increase if task state has a different revision.

At a response counter of 3, the next request gives a task reminder after the conversation messages. The model must use `task_state` before other work. If the definition is not available, the model must use tool discovery first. The task reminder does not stop other tools.

The model can change only the `DOING` object. A task update can also keep the same text. The task reminder stays in model requests until a task update puts the response counter at 0.

The model receives one task state snapshot. The task state snapshot is JSON data, not system instructions. The loop adds it before context and request stages. The context limit includes the task state snapshot and task reminder. The task state snapshot and task reminder are not written to the database.

The task state snapshot and task reminder use a request-local runtime message. A task update does not change the system prompt prefix. Context fitting does not use the runtime message as a new turn.

## Cancellation and Session Deletion

- Cancellation or a turn error removes content from `DOING` and keeps TODO.
- Revert removes task state in the same database operation as the history change.
- Session deletion removes task data. After session deletion, the tool gives an error.
- After a child agent completes a task, the loop removes task state for the child session. Task state for the parent session does not change.
- After you start the harness again, task state and the response counter are available from the database.

Task state has a session ID, revision, and time. The revision increases after the tool completes a task update. The response counter stays internal. A previous API response or event must not replace a newer revision in the UI.

## API and Display

`GET /sessions/{id}/task-state` gives task state. Before the initial task update, a session gives an empty list, `Doing: null`, and revision 0. The revision does not decrease after a task update. A missing session gives `404`.

The event `task_state.updated` gives task state in the payload. The UI shows a progress panel above the composer. The progress panel shows the task items and `DOING` object. Empty task state removes the progress panel.
