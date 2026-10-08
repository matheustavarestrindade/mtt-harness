# Spaced Repetition Checks

## Plugin

- [x] The plugin reads `low`, `medium`, and `high` prompt templates.
- [x] The bootstrap file keeps paths. The database keeps plugin settings.
- [x] Context growth uses reminder patterns.
- [x] Reminders keep the same text.
- [x] Model input limits and context limits include reminders.
- [x] Instruction recovery uses memory search at compression level `M`.
- [x] Workspace usage includes recovery workers.
- [x] The plugin rejects previous worker data after a revert operation.
- [x] The UI gives workspace configuration and usage.

## Checks

- [x] The test suite examines reminder checkpoints and context removal.
- [x] The test suite examines compression level `M` without a snapshot change.
- [x] The test suite examines cancellation, session deletion, and worker data.
- [x] The test suite examines provider payloads.
- [x] The test suite examines UI controls on large and small screens.
- [x] Live model requests complete the instruction recovery checks.
