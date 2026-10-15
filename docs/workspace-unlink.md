# Unlink a Workspace

The unlink operation removes a workspace reference from the harness. Files, directories, symbolic links, and file contents stay the same. The workspace directory is not necessary for the operation.

## API

Send `POST /instances/{id}/unlink`. A request body is not necessary. Authentication is necessary. Status `200` gives the response:

```json
{"id":"INSTANCE_ID","status":"unlinked"}
```

An active conversation, child agent, message in the queue, or active process gives `409`. The work continues. The API gives `404` when the workspace reference is not available.

The database keeps sessions, messages, task state, plugin data, settings, keys, and usage. The workspace list does not include the unlinked workspace. The next harness start does not start the unlinked workspace. The previous instance ID cannot start again.

The route `DELETE /instances/{id}` keeps the workspace reference. The start route can start a stopped workspace. A new workspace request for the same directory makes a new instance ID.

## Operation Control

The queue directory controls the instance lifecycle gate. The instance lifecycle gate controls `stop`, `start`, and `unlink`. Workers read and write data. Session coordinators control message admission. The loop and process manager prevent new work during the unlink operation.

After an error, the previous admission mode is available again. The database field `unlinked` prevents a new workspace reference with the previous instance ID. A new session cannot use the previous instance ID.

After the harness accepts the request, the unlink operation can continue if the client connection closes. The harness waits for the unlink operation before it closes storage.

## UI

Open the workspace menu. Use `Unlink workspace` to remove the workspace reference. The dialog shows the workspace path. The UI removes the workspace reference after the API accepts the operation.

The UI can select a different workspace after the unlink operation. If a different workspace is not available, the UI shows an empty workspace list. The URL changes with the selection. A previous API response cannot put the unlinked workspace back in the list. Message drafts for different workspaces stay the same.
