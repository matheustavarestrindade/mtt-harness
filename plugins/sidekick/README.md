# Sidekick Plugin

## Purpose

Sidekick gives the primary agent context for the selected task. The default plugin state is `OFF`. The plugin uses harness interfaces for task state, memory search, workspace files, model requests, and usage data.

The `sidekick` Postgres schema keeps session data, worker records, context text, and usage counters. The plugin does not change workspace memory or conversation messages.

## Task State

The plugin receives `task_state.updated` events. The event handler only changes local data. Workers read files and send model requests.

The worker examines `DOING.Title` and `DOING.Description`. A model request is not necessary for a different TODO status. A new task replaces the previous task. The worker stops when the DOING data is empty.

A new model request also examines task state. The plugin keeps worker records when the harness stops. A previous user request, task, or configuration cannot supply a new Sidekick note. The plugin does not send the same task to a model worker again after a satisfactory result.

## Source Data

Memory retrieval uses the `medium` compression level. Memory retrieval does not change the memory snapshot or compression levels. Memory versions are examined again before a Sidekick note is used.

File retrieval compares task text with workspace files. It gives text with paths, line ranges, and SHA-256 values. File retrieval stays in the workspace. Symbolic links, credential files, dependencies, and image build output are not applicable retrieval sources.

```text
File search bounds
Entries examined:       4096
Files examined:          256
Total bytes read:        2 MiB
Maximum file size:       64 KiB
Maximum result count:    12
Maximum result text:     16384 UTF-8 bytes
Maximum one excerpt:     4096 UTF-8 bytes
```

File retrieval has limits. The plugin does not examine the full workspace. The plugin rejects a Sidekick note if a retrieval source has a different version. The plugin does not run shell commands or change files.

## Context Data

A model request is necessary only when file retrieval or memory retrieval gives new data. The worker receives the task, user request, conversation text, and retrieval sources. The worker can give an empty result. A Sidekick note must refer to a supplied source.

The worker also receives text from the selected model request. The text includes the memory snapshot. The same memory text does not start a model request again.

The primary agent continues while the worker runs. A Sidekick note is supplied in the next model request. The harness completes the tool group before the request. The input limit includes the Sidekick note. The plugin keeps the Sidekick note until a model request has sufficient space.

```text
<sidekick>
Task-scoped reference data and a quoted JSON note.
</sidekick>
```

The plugin writes a Sidekick note with a conversation message ID. Subsequent requests keep the same text and position. Context removal also removes a Sidekick note when the message ID is not in the request.

The provider receives the Sidekick note as runtime data. The startup prompt does not change. The plugin does not add user messages or start more turns. The plugin does not keep a Sidekick note as `assistant` text. The primary agent uses the information without a report about the worker process.

## Configuration

Settings use the database key `plugin.sidekick`. Harness settings give defaults. Workspace fields can override harness values. JSON `null` removes a workspace value. A worker model ID in the format `provider/model` is necessary before `ON`.

```json
{
  "enabled": true,
  "worker_model": "provider/model",
  "worker_effort": "",
  "memory_enabled": true,
  "files_enabled": true,
  "debounce_ms": 750,
  "cooldown_ms": 15000,
  "job_timeout_ms": 45000,
  "worker_count": 1,
  "worker_output_tokens": 768,
  "memory_limit": 5,
  "memory_bytes": 6000,
  "file_limit": 5,
  "file_bytes": 8000,
  "source_bytes": 4000,
  "hint_bytes": 1600
}
```

The time unit is milliseconds. A value of `0` removes the selected interval. The timeout has a minimum value of `1000`. Byte limits use UTF-8 bytes. The `Schema.JSON` field gives parameter descriptions, defaults, and limits.

The bootstrap file field `sidekick_prompt_file` selects the worker prompt template. The default file is `plugins/sidekick/prompt.md`. The command-line argument `--sidekick-prompt-file` replaces the file value. The harness reads the template one time when the program starts.

## Usage and Worker Control

Worker usage uses the agent name `sidekick.context`. The core usage store keeps provider costs, cost estimates, cache data, and unknown prices. Workspace usage includes worker model calls. The input for a Sidekick note is part of session usage.

The plugin gives usage counters for retrieval sources and model calls. The statistics API does not give the text from a Sidekick note.

Cancellation stops the worker. Revert and session removal stop the related workers before the history change. Session removal removes worker records but keeps tombstones and workspace usage. The harness closes dependencies after the workers stop.

## UI and API

The `Sidekick` section is in the settings dialog. The task panel also has a `Sidekick settings` button. The UI shows workspace configuration and usage.

```text
GET/PATCH /plugins/sidekick/settings
GET/PATCH /instances/{id}/plugins/sidekick/settings
GET       /instances/{id}/plugins/sidekick/statistics
```

An API error stays in the plugin panel. A response from a different workspace cannot change the selected view.
