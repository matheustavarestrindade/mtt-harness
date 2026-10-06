package tools

func (TaskState) SearchDocument() string {
	return `Track multi-step work and show user progress with persistent TODO and DOING. task_state can create task items, update an item by ID to in_progress or done, cancel obsolete work, and replace the current activity title and description. Update TODO and DOING together or independently without resending the whole list. Completed items remain while work is unfinished; final completion clears both fields. State belongs to the current session and survives restarts. Use it only when tracking is useful. Model-request reminders appear after three responses without a successful update. Retrieve the schema before calling the tool.`
}

// Usage documents supplement descriptions and input schemas in the internal
// embedding index. They are separate from provider requests and tool results.
func (bashTool *Bash) SearchDocument() string {
	return `Use this tool to execute shell commands, run scripts, compile a project, or launch a development server in the instance workspace.
For a short command, call {"command":"go test ./..."}; the tool waits and returns stdout and stderr once as its tool result, with no separate process notification.
For a long-running server, call {"command":"npm run dev","wait":false}; then use process_output to read logs or process_kill to stop it. Background notification modes are none, exit (default), error, and interval. They are runtime updates, not new user requests.`
}

func (FileActions) SearchDocument() string {
	return `Discover file_actions to inspect files, list directories, glob file paths, create files, write selected lines, replace literal text, append, prepend or delete. It is not preloaded; its complete definition is supplied after search_tool finds it.
Supply path for one target or paths for up to 32 targets. Both inputs are mutually exclusive. One shared actions array and return selection apply to every target, saving repeated content and arguments.
Replace across files: {"paths":["src/a.go","src/b.go"],"actions":[{"op":"replace","old_text":"oldName","new_text":"newName","mode":"all"}],"return":{"type":"summary"}}.
Append the same text: {"paths":["one.txt","two.txt"],"actions":[{"op":"append","content":"endfile"}],"return":{"type":"summary"}}. All file operations, including delete and list, accept paths.
Resolve and prepare all targets before commit. Duplicate resolved paths and overlapping mutation paths fail. Canonical path gate order avoids deadlocks. Preparation failures make no changes; filesystem commits are atomic per target. A commit error stops later targets and reports previously committed target indexes, so the model can account for partial completion. No automatic retry or rollback follows a commit failure.
Read a range: {"path":"src/main.go","actions":[{"op":"read","start_line":10,"end_line":40}]}.
Find source files recursively: {"path":".","actions":[{"op":"glob","pattern":"**/*.{go,md}","exclude":["**/.git/**","**/vendor/**"]}]}.
Glob matches relative paths with Doublestar syntax: ** for any directory depth, * and ?, character classes and brace alternatives. It defaults to regular files; kind directory/link/all selects other entries. Hidden entries are included; .gitignore is not read. Excluded directories are pruned. Glob never traverses symlinks, including literal prefixes. Results are compact F/D/L/S quoted relative paths with optional listing fields. The default page is 100 matches, maximum 200 within the shared output budget. Continue with the returned cursor and unchanged root/query. Glob discovers paths only; choose returned paths explicitly for later reads or multi-file edits.
List direct children: {"path":"src","actions":[{"op":"list","limit":100}]}. Default rows are F "file.txt", D "directory", L "link", or S "special". No sizes or other metadata appear unless requested. Add fields:["permissions","owner","group","size","modified"] for the octal mode, numeric UID/GID, byte size, and UTC modification time, in the supplied order. Follow the returned opaque cursor for another directory page.
Without return, read/list actions emit only their data. With return, only selected final output is returned. Intermediate reads/lists are suppressed and do not consume the final preview budget. Single-target output has no banner, action log or totals. Multiple-target read/list data has short path labels, diffs identify their own paths, and summary combines status counts. Output shares 200 lines/16 KiB across targets; a next target index identifies omitted later targets.
Chain known edits in order on one staged file, then commit once: {"path":"notes.txt","actions":[{"op":"write","content":"hello\n"},{"op":"append","content":"world\n"}],"return":{"type":"read"}}.
Append fully specified text and inspect the result without a preliminary read: {"path":"log.txt","actions":[{"op":"append","content":"\nNext entry\n"}],"return":{"type":"read"}}. Append/prepend are literal and add no implicit separators. Read an edge first only if unknown formatting or syntax changes the required text.
For edit-and-inspect, use only mutations and return.type=read. A trailing read is unnecessary; only return output is shown. Edits require an explicit output choice. return summary gives Created, Updated, Deleted, Unchanged or OK. return diff with context_lines selects only the net unified diff; return read selects only final text using start_line/end_line. A diff is never automatic. Returned previews share 200 lines/16 KiB with exact continuation cursors.
Replace known text: {"path":"src/main.go","actions":[{"op":"replace","old_text":"oldName","new_text":"newName","mode":"all"}],"return":{"type":"diff","context_lines":3}}.
Delete an entry: {"path":"obsolete.txt","actions":[{"op":"delete"}],"return":{"type":"summary"}}. Regular files can be deleted within a staged file chain; final reads require a file still to exist. Directories and symbolic links require standalone delete with summary. Directory deletion is empty-only, never recursive. Symlink deletion removes the link, never its target. The instance workspace root cannot be deleted. File deletion with return diff shows the old content against /dev/null; it requires permission to read that content. Summary deletion does not read file contents.
The first failure stops further execution. No diagnostic read/list or retry runs. Preparation failures discard all staged edits; commit failures report committed target indexes. Errors contain the failed operation, cause and observed file modification time when available. Missing-text errors include a bounded excerpt. Reuse content from instructions, prior reads and edit previews. Known exact replacements can be tried directly. Read first only when missing content affects the edit. A preplanned chain cannot make new model decisions from intermediate reads.`
}

func (Read) SearchDocument() string {
	return `Use this tool to inspect the contents of an existing file in the workspace.
Call {"path":"src/main.go","start_line":10,"end_line":40} to read a selected range without loading the whole file.
Omit the range to start at line 1. Output stops at 200 lines or 16 KiB. A truncation notice gives the exact next start_line and, for a long line, a zero-based start_byte within that line. Use that cursor to read the rest without repeated or missing bytes.`
}

func (Write) SearchDocument() string {
	return `Use this tool to create a file or save new content to an existing file.
Call {"path":"notes.txt","content":"hello\n"} to write the entire file.
Provide start_line and end_line to replace or delete complete selected lines while preserving surrounding content.
Success reports the path, creation/change/no-op status, line and byte counts, and a bounded preview from this edit's own snapshot. A preview limit never truncates the file.
To inspect the result in the same call, add "return":{"type":"lines","start_line":10,"end_line":40}, or "return":{"type":"file"}. These modes show updated-file text with absolute line-number labels, capped at 200 lines or 16 KiB and followed by a read continuation cursor if needed.
Use "return":{"type":"diff"} for changed lines only, or "return":{"type":"surrounding","size":5} for the diff with five unchanged lines above and below each hunk. Omitted return keeps the existing three-context-line diff. Invalid return settings or ranges fail before the file changes.
Use the returned preview instead of immediately reading the same lines again. On a stale edit range error, read the current file and update the bounds before retrying.`
}

func (Replace) SearchDocument() string {
	return `Use this tool to edit a file by replacing exact literal text, including source code and configuration values.
Call {"path":"main.go","old_text":"oldName","new_text":"newName","mode":"all"} to replace every non-overlapping occurrence.
Choose first or last for a single occurrence. Optional line bounds restrict the edit to a selected block.
Success reports the replacement count and mode with a bounded unified diff. A missing match leaves the file unchanged and identifies the missing text and selected range.`
}

func (processOutputTool *ProcessOutput) SearchDocument() string {
	return `Use this tool to inspect logs and execution status for a background command started by bash.
Supply the harness process ID returned by bash, not the operating system PID.
This reads retained stdout and stderr, including output from a completed process.`
}

func (processKillTool *ProcessKill) SearchDocument() string {
	return `Use this tool to stop or interrupt a command or development server started by bash.
Supply its harness process ID and the supported signal.
It signals the process group so shell descendants are stopped with the command.`
}

func (agentTool *Agent) SearchDocument() string {
	return `Use this tool to delegate a self-contained task to a child agent with a model from the instance's available list.
For example, delegate a source-code review or a smaller research task to another model.
The child shares the instance workspace. Its completion result returns to the parent conversation.`
}

func (Finish) SearchDocument() string {
	return `A child agent uses this tool to report its result and mark its delegated task complete.
It ends the child workflow and sends the result to the parent. It is only callable from a child session.`
}

func (searchTool *Search) SearchDocument() string {
	return `Use this tool to discover a capability before its callable definition is in the current tool group.
Describe the desired operation, for example {"query":"shell command exec"}, or request an exact category such as file.
Results identify tools. Their instructions and input schemas are available in the following model request.`
}
