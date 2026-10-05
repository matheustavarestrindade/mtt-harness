# File Actions

## One File Interface

The tool `file_actions` gives file text, file edits, and directory data. The full schema is in the initial model request. It replaces the tools `read`, `write`, and `replace`.

One tool call has one path and an array of 1 to 32 actions. A relative path starts at the instance workspace. Actions run in sequence. An action uses the content from the previous action.

The actions are:

- `read`: read file text, with an optional line range.
- `write`: make or replace a file, or replace full selected lines.
- `replace`: replace text matches with mode `first`, `last`, or `all`.
- `append`: add text at the end of a file.
- `prepend`: add text at the start of a file.
- `list`: show directory entries.

The harness accepts a different path in a different tool call. It can run tools at the same time. The harness puts tool calls for the same resolved path in sequence.

## File Text and Directory Data

Read a line range:

```json
{
  "path": "src/main.go",
  "actions": [
    {"op": "read", "start_line": 10, "end_line": 40}
  ]
}
```

Line numbers start at 1. The range includes the start line and end line. Without a line range, a read action starts at line 1 and continues to EOF. The output limit can stop the text. The last LF does not make a new line. An empty file has 0 lines.

List a directory:

```json
{
  "path": "src",
  "actions": [
    {"op": "list", "limit": 100}
  ]
}
```

The result includes directory entries with a name that starts with `.`. It gives the name, type, and number of bytes for a directory entry. Directory entries are in name sequence with letter case kept.

The tool does not follow symbolic links. The default limit is 100 directory entries, and the maximum is 200. A directory cursor gives the position for the next directory page.

A `list` action cannot be in the same tool call as a file action. The tool call has one file path or directory path.

## Output Types

File content can be available from the user, previous messages, or tool output. Use available content for the file edit. A new read action is not necessary if the information for the file edit is available.

For supplied text at the end or start of a file, use `append` or `prepend`. A `replace` action can use text from the user or previous tool output. The tool examines the text match before the file replacement. Use `return` to examine the new content. Use `on_error` to get error diagnostics if the file edit gives an error.

Read unknown file content only if it changes the file edit decision. This includes file structure, line numbers, and text format. A short preview is not the full file. If the file has changed, get the necessary new content. After the file edit, do the checks necessary for the task.

A file edit must have a `return` object. The tool does not select a unified diff automatically. Select the result necessary for the task:

- `summary`: file status, action information, and the number of lines and bytes.
- `diff`: a unified diff between previous and new content.
- `read`: new file text or a line range.
- `list`: directory entries.

For a unified diff, `context_lines` has a default of 3. The range is 0 to 100. A value of 0 removes context lines.

Make a file and add text in one tool call:

```json
{
  "path": "notes.txt",
  "actions": [
    {"op": "write", "content": "hello\n"},
    {"op": "append", "content": "world\n"}
  ],
  "return": {"type": "read"}
}
```

The `append` and `prepend` actions do not add line breaks automatically. The content must include the necessary line breaks. The file must be available. A previous `write` action without a line range can make the file in the same tool call.

Use `return.type` with the value `summary` when file content is not necessary. Use `read` to examine new text. Do not read the same result again unless more content is necessary.

Read actions give output at a position in the sequence. Output from `return` comes after the sequence. The model must not get the same content again.

## Error Output

The tool uses temporary file content for a file edit. The actions and output checks must be correct before the tool replaces the file. An error prevents the file change. A file edit keeps permission bits. Other hard links keep the previous content.

The optional `on_error.return` gives an error diagnostic for the same path. The type must be `read` or `list`. It cannot change file content or run the actions again.

The error diagnostic uses previous file content. The error status and initial cause stay in the result. An error diagnostic cannot replace the initial error.

```json
{
  "path": "src/main.go",
  "actions": [
    {"op": "replace", "old_text": "oldName", "new_text": "newName", "mode": "all"}
  ],
  "return": {"type": "diff", "context_lines": 3},
  "on_error": {
    "return": {"type": "read", "start_line": 10, "end_line": 50}
  }
}
```

Incorrect input and cancellation do not start error diagnostic I/O. The error diagnostic gives content for the next action. Get more file content only when necessary. An action sequence cannot make a new model decision internally.

## Output Limits

Preview data in one tool call has a total limit of 200 lines or 16 KiB. The limit includes line-number labels. Status and truncation notices do not use the preview limit. The limits do not change file edits.

A file truncation notice gives the next `start_line` and, for a long line, `start_byte`. The byte position starts at 0 within the line. Use the position from the result. It must not divide a UTF-8 character. A directory truncation notice gives a directory cursor for the next list action.

The format applies to actions, `return`, and error diagnostics. Labels and truncation notices are not file content. The setting is `MTT_READ_LINE_NUMBERS`.

The tool removes the same outer text from the diff input but keeps lines of context. The diff input contains previous file content and replacement file content. The maximum total is 256 KiB or 4000 lines. If the diff input is above the limit, the result gives the cause without the preview. Incorrect UTF-8 or NUL bytes also prevent a preview.
