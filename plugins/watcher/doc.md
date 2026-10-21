# Watcher Plugin

## Status

The plugin is on `feat/watcher-plugin`. The harness loop runs watcher sessions.

## Workspace Settings

The database setting is `plugin.watcher`. The default plugin state is `OFF`. A workspace can have many watcher definitions. The workspace UI is at `Settings → Watchers`.

A watcher definition has an ID, name, model, prompt template, watcher triggers, and tool selection. Use `New watcher`, `Edit`, `Duplicate`, or `Remove`. A worker model is necessary for `ON`. The workspace model list applies.

```json
{
  "enabled": true,
  "watchers": [{
    "id": "scope-review",
    "name": "Scope review",
    "enabled": true,
    "model": "provider/model",
    "reasoning_effort": "",
    "triggers": ["context_growth", "permission_request", "end_turn"],
    "interval_tokens": 32768,
    "context_tokens": 32768,
    "include_reasoning": false,
    "outside_workspace_only": true,
    "permission_tools": ["file_actions"],
    "tools": [{"name": "file_actions", "operations": ["read", "list", "glob"]}],
    "timeout_ms": 120000,
    "max_rounds": 12,
    "max_output_tokens": 2048,
    "max_continuations": 3
  }]
}
```

The configuration schema gives defaults. A workspace watcher array replaces the harness watcher array.

```text
Missing or zero numeric setting: use its default
Missing or empty prompt: use the default template
```

## Watcher Triggers

- `context_growth`: The plugin starts a watcher run after the token interval. The interval uses context growth, not `Statistics.Input`. Context removal or a model change starts a new interval.
- `permission_request`: The watcher examines a permission request before the tool runs. The watcher definition has `permission_tools` and `outside_workspace_only` fields.
- `end_turn`: `EndTurn` starts a watcher run after a primary turn with status `completed`. A runtime continuation can start a new primary turn through the runtime queue.

Watcher sessions and the child sessions of a watcher cannot start watcher runs. A primary turn started by a runtime continuation stays a primary turn.

## Tool Access

A watcher run has a worker session and an execution profile. The execution profile cannot change. The execution profile applies to `search_tool` and tool calls. The user can select file operations for `file_actions`.

```text
Initial tool choices: read, list, glob, search_memory, list_memory_categories
Fixed decision tools: continue, interfere
Fixed discovery tool: search_tool
```

The context plugin is necessary for memory tools. Tool grants do not use the permission pipeline. The harness examines input and model limits. Cancellation applies. A tool grant does not make a workspace approval.

Child agents keep the same tool selection and use `finish`. Worker sessions are not in the primary conversation list.

## Decisions

The tool `continue` stops a watcher run. For a permission review, the decision is `allow` with `scope:once`. For other watcher triggers, the primary agent can continue.

The tool `interfere` stops a watcher run and gives a correction. For a permission review, the decision is `deny`. For a completed primary turn, the correction becomes runtime input for a new turn. The correction is not a user message.

The startup prompt tells the agent to use watcher decisions for the user's task. The user request controls the work.

A permission request has approval only if the watcher decisions are `continue`. An `interfere` decision gives `deny`. An error status cannot give approval. A user decision cannot be changed by a subsequent watcher decision. The queue controls the correction and the permission decision in one operation.

## Template Variables

```text
{todo}                    TODO items with IDs and statuses
{doing}                   DOING title and description
{event}                   trigger metadata
{workspace}               workspace path
{permission_request}      pending permission details, or null
{tool_call}               exact tool call, or null
{last_context}            configured default context window
{last_32k_context}        explicit 32768-token window
{last_65536_context}      explicit token window
{{variable}}              literal {variable}
```

Variable values are not examined for more variables. Conversation and task data are not system instructions for the watcher. The source does not include `ProviderState`, `Content.Data`, or `data:` URLs. The `input_tokens` field gives the token estimate. The `InputTruncated` value is `true` if content is not included.

The input starts with `<watcher_event>` metadata.

```text
source_phase: active_request | pending_permission | completed_turn
in_progress_message_id: current response ID, when available
InProgress: true marks a partial transcript response
Empty streaming placeholders are omitted
Event metadata is supplied even when {event} is absent from the template
```

```text
Review progress against the user's instructions.
Use memory or file reads when needed.
Call continue if the normal flow can proceed.
Call interfere with a concrete correction otherwise.

<todo>{todo}</todo>
<doing>{doing}</doing>
<trigger>{event}</trigger>
<conversation_to_analize>
{last_context}
</conversation_to_analize>
```

## Limits

```text
Context source: at most 1024 messages and 4 MiB of public text
InputPreview in activity: at most 16384 bytes
Input preview action: at most 65536 bytes
Worker pool: 4 review groups
Pending review groups: at most 1024
Activity page: default 50, maximum 100
Timeout: default 120000 ms; range 1000-900000 ms
Model rounds: default 12; range 1-100
Output reservation: default 2048 tokens; range 128-65536
Automatic interventions: default 3 per watcher and user-instruction chain
Runtime correction: at most 65536 bytes
```

The `max_continuations` limit applies across primary turns. After new user input, the `continuations` value is `0`. A watcher run with status `limited` identifies the limit. The `timeout_ms` value applies only to a watcher run. The harness waits for user permission without a timeout.

## Data and Cancellation

The database keeps watcher definitions in `plugin.watcher`. The `watcher` Postgres schema keeps watcher schedules, decisions, and watcher run records. Event callbacks send metadata to workers. Workers do I/O and use the harness loop. Session coordinators do not run plugin callbacks or models.

The plugin examines task state, user input, configuration, history, and permission IDs before it uses a decision. The harness uses a source fence before it applies a decision. Cancellation, `revert`, and `delete` stop previous work.

`EndTurn` schedules a watcher run. `EndTurn` does not wait for the watcher result. The watcher uses a different cancellation context. Runtime continuation uses the session queue and keeps the source watcher ID. The queue starts a new turn after the previous turn stops.

On program start, a watcher run with status `running` gets status `interrupted`. The plugin does not apply a decision with `interrupted` status. Session deletion removes watcher run records. The `revert` operation keeps only watcher run records with a reminder anchor in the session history. A configuration change keeps previous watcher run records.

## Usage and Watcher History

Workspace usage includes watcher model calls. The agent name is `watcher.ID`. Primary conversation cost does not include worker usage. Input tokens from a runtime continuation are part of primary request usage.

The UI gives watcher decisions and an input preview through disclosure controls. The `SessionActivity` API does not give `Message.Reasoning` from worker sessions. The API uses activity pages and history revisions.

## API

```text
GET   /instances/{id}/plugins/watcher/settings
PATCH /instances/{id}/plugins/watcher/settings
GET   /instances/{id}/plugins/watcher/statistics
POST  /instances/{id}/plugins/watcher/actions/preview
POST  /instances/{id}/plugins/watcher/actions/runs
POST  /instances/{id}/plugins/watcher/actions/cancel
GET   /sessions/{id}/plugins/watcher/activity
```

The settings schema includes the tool catalog and default watcher definition. The preview action accepts `session_id`, `watcher`, and `trigger`. It measures input without a model call.

The `runs` operation accepts `session_id`, `cursor`, and `limit`. An empty session ID selects watcher run records from the workspace. The `cancel` operation accepts `run_id` and cancels the review group.

Session events include `plugin.activity` metadata. The UI uses the events to refresh watcher run records. A runtime continuation uses the same watcher run record in the conversation view.

## Test Suite

The test suite includes tool access, model request definitions, permission decisions, runtime continuation, cancellation, and database data. The UI test suite includes watcher settings and watcher run records.

```sh
go test -race ./plugins/watcher ./internal/organism/loop ./internal/molecule/store/...
cd ui
npm run check
npm run test:e2e -- tests/watchers.spec.ts
```

The full test suite is necessary.
