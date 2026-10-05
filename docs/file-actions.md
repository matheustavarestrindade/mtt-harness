# File Actions

## One File Interface

The tool `file_actions` gives file text, file edits, and directory data. Use `search_tool` to get the full definition before the initial file action. The next model request has the full schema. A definition that is available can be used again.

One tool call has `path` or an array of 1 to 32 `paths`. The input must not have the 2 fields together. One array of 1 to 32 actions applies to the paths. A relative path starts at the instance workspace. An action uses the content from the previous action for the selected path.

The actions are:

- `read`: read file text, with an optional line range.
- `write`: make or replace a file, or replace full selected lines.
- `replace`: replace text matches with mode `first`, `last`, or `all`.
- `append`: add text at the end of a file.
- `prepend`: add text at the start of a file.
- `delete`: remove a file, symbolic link, or empty directory.
- `list`: show directory entries.

The harness accepts a different path in a different tool call. It can run tools at the same time. The harness puts tool calls for the same resolved path in sequence.

## Path Batches

The model can supply text and actions one time for a batch of files:

```json
{
  "paths": ["src/a.go", "src/b.go"],
  "actions": [{"op":"replace","old_text":"oldName","new_text":"newName","mode":"all"}],
  "return": {"type":"summary"}
}
```

The same actions and output selection apply to the paths, in input sequence. The actions can be `append`, `prepend`, `write`, `delete`, `read`, or `list`. The input paths must resolve to different paths. Paths for file edits must not contain other selected paths.

The tool prepares content, output, and temporary files for the paths before the initial commit. An error before the initial commit prevents file edits. A path has one commit. A commit error stops new file edits. Files from previous commits do not change after the error.

The error gives the path with the error and the target indexes for previous commits. Target indexes start at 1 in the input array.

For a batch, a `read` or `list` result has a short path label. A unified diff gives the file path. A `summary` result gives status counts, such as `Updated 2`. One output limit applies to the paths together. A truncation notice can give the next target index.

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

The result includes directory entries with a name that starts with `.`. The default result gives only the type and name. `F` identifies a file, `D` a directory, `L` a symbolic link, and `S` a different entry type. Directory entries are in name sequence with letter case kept.

```text
F "file2.txt"
D "test-folder"
```

The optional `fields` array selects more file data. An empty array gives the default result. The result keeps the field sequence from the input:

- `size`: the number of bytes from the file system. A directory value is not the total of the directory contents.
- `permissions`: the octal mode, with special permission bits.
- `owner`: the UID.
- `group`: the GID.
- `modified`: the file modification time in UTC.

A value of `?` identifies data that is not available. The fields give file data, not permission requests to the user. The tool gets file data only for the selected fields. The same `fields` parameter is available with `return.type` set to `list`.

```json
{"path":"src","actions":[{"op":"list","fields":["permissions","owner","size"]}]}
```

The tool does not follow symbolic links. The default limit is 100 directory entries, and the maximum is 200. A directory cursor gives the position for the next directory page.

A `list` action cannot be in the same tool call as a file action. The tool call has one file path or directory path.

## Output Types

File content can be available from the user, previous messages, or tool output. Use available content for the file edit. A new read action is not necessary if the information for the file edit is available.

For supplied text at the end or start of a file, use `append` or `prepend`. A `replace` action can use text from the user or previous tool output. The tool examines the text match before the file replacement. Use `return` to examine the new content.

Read unknown file content only if it changes the file edit decision. This includes file structure, line numbers, and text format. A short preview is not the full file. If the file has changed, get the necessary new content. After the file edit, do the checks necessary for the task.

A file edit must have a `return` object. The tool does not select a unified diff automatically. Select the result necessary for the task:

- `summary`: one status: `Created`, `Updated`, `Deleted`, `Unchanged`, or `OK`.
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

When `return` is in the input, the result contains only the output from `return`. The tool does not show output from a read action or list action. For one path, it does not add a path, action data, number of lines or bytes, or output label. The output limit applies to `return` only.

To add text and examine the new file, put only the file edit in `actions`. The output type is `read`. Do not add a read action for the same new content. For a read action without file edits, `return` is not necessary.

```json
{
  "path": "notes.txt",
  "actions": [{"op": "append", "content": "endfile"}],
  "return": {"type": "read"}
}
```

Without `return`, the tool gives selected file text or directory data in action sequence. One preview limit applies to the full output. A file edit must have `return`.

## File Removal

```json
{"path":"obsolete.txt","actions":[{"op":"delete"}],"return":{"type":"summary"}}
```

The `delete` action removes a file, symbolic link, or empty directory. It does not remove the contents of a directory. The tool does not follow symbolic links. The tool cannot remove the instance workspace root. If the path is not available, the tool gives an error.

A directory or symbolic link removal must use one `delete` action and a `summary` result. File removal can be in a sequence with different file actions. The new content is temporary until the actions and selected output are correct. A subsequent `write` action without a line range can make the file again.

For a file, `return.type` can be `diff` to show the previous text with `/dev/null` as the new path. The tool must read the file for a unified diff. A `summary` removal does not read file content. A file text preview after removal gives an error and prevents the removal.

## Error Output

The tool uses temporary file content for a file edit. The actions and output checks must be correct for the paths before the initial commit. An error before the initial commit prevents file edits. A replacement keeps permission bits. Other hard links keep the previous content.

The initial error stops the tool call. The tool does not start the next action or prepare more output. It does not read the file or directory again. Temporary file content and previews are discarded. An error from the commit gives the target indexes for previous commits. Files from previous commits do not change after the error.

The result gives only the operation, action number, path, and cause of the error. The tool does not give steps to correct the error. The model selects the next action.

The result gives a preview of `old_text` if a text match is not available. A long value has a maximum preview of 160 bytes. The result can also give the file modification time. The time comes from file data available before the error. Error output does not cause a new file operation.

```json
{
  "path": "src/main.go",
  "actions": [
    {"op": "replace", "old_text": "oldName", "new_text": "newName", "mode": "all"}
  ],
  "return": {"type": "diff", "context_lines": 3}
}
```

The tool does not accept `on_error`. Incorrect input and cancellation give only the error. An action sequence cannot make a new model decision internally.

## Output Limits

Preview data in one tool call has a total limit of 200 lines or 16 KiB. The limit includes line-number labels. Status and truncation notices do not use the preview limit. The limits do not change file edits.

A file truncation notice gives the next `start_line` and, for a long line, `start_byte`. The byte position starts at 0 within the line. Use the position from the result. It must not divide a UTF-8 character. A directory truncation notice gives a directory cursor for the next list action.

The format applies to actions and `return`. Labels and truncation notices are not file content. A truncation notice gives a position, not a new instruction. The setting is `MTT_READ_LINE_NUMBERS`.

The tool removes the same outer text from the diff input but keeps lines of context. The diff input contains previous file content and replacement file content. The maximum total is 256 KiB or 4000 lines. If the diff input is above the limit, the result gives the cause without the preview. Incorrect UTF-8 or NUL bytes also prevent a preview.
