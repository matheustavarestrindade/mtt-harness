# Spaced Repetition Plugin

## Purpose

The plugin gives reminders after context growth. The default plugin state is `OFF`. The plugin uses harness interfaces for prompt templates, model requests, memory search, and usage data.

The Postgres schema is `spaced_repetition`. It keeps reminder schedules, reminder text, recovery records, and usage counters. Conversation messages stay in the core store. Workspace memory stays in the context plugin.

## Reminder Files

The directory has 3 Markdown files:

```text
plugins/spaced_repetition/prompts/low.md
plugins/spaced_repetition/prompts/medium.md
plugins/spaced_repetition/prompts/high.md
```

The harness reads the files one time when the program starts. The bootstrap file field `spaced_repetition_prompts` selects a different directory. The command-line argument `--spaced-repetition-prompts` replaces the file value. Relative paths use the process directory.

The templates use the template renderer. The template renderer supplies session variables, the tool catalog, and tool information. A template does not activate a tool. See `../../docs/start-prompt.md`.

The provider input contains reminder text:

```text
<spaced repetition>
Rendered reminder text.
</spaced repetition>
```

The plugin writes reminder text with a conversation message ID. Subsequent requests keep the same text at the same position. Context removal also removes a reminder when the message ID is not in the request. A template change removes previous reminder text from the request view.

The Responses API keeps reminders in the conversation input. The `instructions` field does not change because of a reminder. The plugin does not put reminders in `assistant` text or conversation messages.

## Reminder Schedule

The plugin measures the selected request after context compaction. Reminder text does not increase context growth. The provider can give a token count. Other token counts are token estimates.

```text
Default adaptive interval:
  minimum: 256 tokens
  interval: model context capacity * 0.125
  maximum: 32768 tokens
Default pattern:
  low, low, low, medium, repeat
Example with 262144-token capacity:
  32768 -> low
  65536 -> low
  98304 -> low
  131072 -> medium
```

A request below the next reminder checkpoint does not add a reminder. Context growth can include 2 or more reminder checkpoints. The plugin gives one reminder. If a `medium` reminder checkpoint is included, the reminder uses `medium`.

Context removal and a model change set the next reminder checkpoint. The reminder pattern position stays the same. A smaller token estimate without context removal does not start a new reminder schedule. A revert operation keeps the reminder pattern position for reminders that stay in the history.

The context limit includes reminder text. The context plugin can supply a smaller input limit. The plugin keeps a reminder until a model request has sufficient space. The plugin changes the reminder schedule when the harness sends the model request. After a provider error, a subsequent model request can use the same reminder.

## Instruction Recovery

Tool discovery gives the `remember_instructions` definition. The input is:

```json
{"reason":"The user repeatedly corrects my stack, database, writing style, and confirmation behavior."}
```

The `reason` field must have 1 to 4096 UTF-8 bytes. A worker model is necessary. The tool waits for a recovery report. It gives `{"status":"ready"}` after the recovery report is in the database.

The primary model receives the recovery report at a subsequent request boundary. The harness must complete the tool group before the request.

The worker has 2 model requests. Request 1 selects memory queries. Request 2 makes a recovery report from instruction sources. The instruction sources include the startup prompt, user messages, and memory records.

Memory search uses compression level `M`. It selects workspace memory records without a change to the memory snapshot. Memory text does not replace a user instruction or a system instruction. The recovery report must use supplied instruction sources.

If memory is not available, the worker uses the other instruction sources. A model or database error stops the worker. A completed tool call ID uses the previous result. It does not send model requests again.

A `high` reminder can replace an automatic reminder at the same request boundary. The reminder schedule includes the reminder checkpoints used by the `high` reminder. Instruction recovery without an automatic reminder does not change the reminder pattern position.

## Configuration and API

Settings use the database key `plugin.spaced_repetition`. A workspace field can override a harness field. A JSON `null` removes the workspace value. The rule also applies to fields in the interval object.

```text
GET   /plugins/spaced_repetition/settings
PATCH /plugins/spaced_repetition/settings
GET   /instances/{id}/plugins/spaced_repetition/settings
PATCH /instances/{id}/plugins/spaced_repetition/settings
GET   /instances/{id}/plugins/spaced_repetition/statistics
```

```json
{
  "enabled": true,
  "interval": {
    "mode": "model_fraction",
    "fraction": 0.125,
    "max_tokens": 32768
  },
  "pattern": ["low", "low", "low", "medium"],
  "worker_model": "provider/model",
  "worker_effort": ""
}
```

Mode `tokens` uses the constant value in `interval.tokens`. Automatic reminders can use an empty worker model. The instruction recovery tool is not available without a worker model. The API schema gives parameter limits and defaults. The worker uses workspace model permissions and provider credentials.

The default instruction recovery limits are:

```text
worker_output_tokens: 1024 per model request
job_timeout_ms: 120000 milliseconds
max_queries: 4
memory_result_limit: 5 per query
memory_bytes: 12000 UTF-8 bytes across deduplicated records
source_bytes: 8000 UTF-8 bytes of recent user text
worker_count: 2 per workspace
active recoveries: 1 per session
```

The configuration applies after active tool groups. A child agent uses the request lease of the parent session when the parent session waits for the child agent. Sessions have different reminder schedules.

## Usage and Storage

Automatic reminders do not send worker model requests. Reminder input tokens are part of session usage. The plugin keeps a token count and identifies token estimates. It does not calculate token cost again.

Instruction recovery uses the workspace agent name `spaced_repetition.instruction_recovery`. Usage has a workspace ID, agent name, run ID, and model request ID. A source session ID identifies instruction sources only. Provider cost, cost estimates, unknown prices, and subscription billing rules stay applicable.

Usage counters include `low`, `medium`, `high`, and `deferred` reminders. The API also gives reminder checkpoints, recovery records, memory search data, and time values. The UI shows configuration and usage in `Settings` -> `Instructions`. A button in the Memory panel opens the same section.

Cancellation stops recovery workers. The plugin rejects a recovery report after new user data, a configuration change, the revert operation, or session deletion. Session deletion removes reminder data and recovery records and keeps a tombstone. Usage records stay in the core store.

After the harness starts again, reminder schedules and recovery reports stay available. Previous model work does not start automatically. A recovery record has status `interrupted` after the time limit. The harness waits for workers before it stops memory, providers, or storage.

## Checks and Removal

```sh
go test -race ./plugins/spaced_repetition ./internal/organism/plugins ./internal/molecule/provider
docker compose --profile test run --build --rm test
```

The value `enabled: false` stops new reminders. Reminder text stays in the request view until the reminder anchor is removed. The context plugin keeps the memory index.

The harness connection is in `cmd/mtt/spaced_repetition.go`. Remove the connection and `plugins/spaced_repetition/` to remove the plugin. Keep general plugin interfaces if other plugins use them. Keep the database content unless the user gives a data removal request.
