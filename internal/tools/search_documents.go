package tools

// Usage documents supplement descriptions and input schemas in the internal
// embedding index. They are separate from provider requests and tool results.
func (bashTool *Bash) SearchDocument() string {
	return `Use this tool to execute shell commands, run scripts, compile a project, or launch a development server in the instance workspace.
For a short command, call {"command":"go test ./..."}; the tool waits and returns stdout and stderr once as its tool result, with no separate process notification.
For a long-running server, call {"command":"npm run dev","wait":false}; then use process_output to read logs or process_kill to stop it. Background notification modes are none, exit (default), error, and interval. They are runtime updates, not new user requests.`
}

func (FileActions) SearchDocument() string {
	return `Use file_actions to inspect files, list directories, create files, write selected lines, replace literal text, append or prepend content.
Read a range: {"path":"src/main.go","actions":[{"op":"read","start_line":10,"end_line":40}]}.
List direct children: {"path":"src","actions":[{"op":"list","limit":100}]}. Follow the returned opaque cursor for another directory page.
Chain known edits in order on one staged file, then commit once: {"path":"notes.txt","actions":[{"op":"write","content":"hello\n"},{"op":"append","content":"world\n"}],"return":{"type":"read"}}.
Append fully specified text and inspect the result without a preliminary read: {"path":"log.txt","actions":[{"op":"append","content":"\nNext entry\n"}],"return":{"type":"read"}}. Append/prepend are literal and add no implicit separators. Read an edge first only if unknown formatting or syntax changes the required text.
Edits require an explicit output choice. return summary gives compact status without file contents; return diff with context_lines selects a net unified diff; return read selects final text using start_line/end_line. A diff is never automatic. Read actions and return output share 200 lines/16 KiB with exact continuation cursors.
Recover with context: {"path":"src/main.go","actions":[{"op":"replace","old_text":"oldName","new_text":"newName","mode":"all"}],"return":{"type":"diff","context_lines":3},"on_error":{"return":{"type":"read","start_line":10,"end_line":50}}}.
If any action or final preview fails, no edits commit. Error recovery is read-only and reports the original snapshot and failure. Reuse content from instructions, prior reads, edit previews and diagnostics. A known exact replacement can be tried directly; its match check and on_error preview provide correction context if necessary. Only read first when missing content affects the edit. A preplanned chain cannot make new model decisions from an intermediate read.`
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
Success reports the replacement count and mode with a bounded unified diff. A missing match leaves the file unchanged and explains how to read it again, copy exact text without display line numbers, and correct stale bounds.`
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
