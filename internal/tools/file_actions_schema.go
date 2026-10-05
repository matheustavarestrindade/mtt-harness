package tools

import "github.com/matheustavarestrindade/mtt-harness/atom"

func (FileActions) InputSchema() atom.Schema {
	return schema(`{
		"type":"object", "additionalProperties":false,
		"properties":{
			"path":{"type":"string","minLength":1,"description":"One file or directory for the whole call, relative to the instance workspace or absolute. Existing symlink ancestors are resolved before path checks. Parent directories must exist. Use separate calls for different paths."},
			"actions":{"type":"array","minItems":1,"maxItems":32,"description":"1-32 actions, executed in order on the same path. Every action sees the preceding action's result, including shifted line numbers. File mutations commit once only if all actions and the selected output succeed. list actions cannot mix with file actions. Do not insert a preparatory read when the needed content or literal edit is already known. Use return to inspect the edited result; use on_error for missing context after failure. Read actions explicitly request output and share the call's preview budget.","items":{
				"type":"object","additionalProperties":false,"required":["op"],"properties":{
					"op":{"type":"string","enum":["read","write","replace","append","prepend","list"],"description":"read inspects file text; write creates/replaces a file or selected lines; replace changes exact text; append/prepend add literal content at the end/start; list enumerates direct directory children. append/prepend/replace/read require an existing file or an earlier whole-file write in the chain."},
					"content":{"type":"string","description":"Required for write, append and prepend only. Literal text without display labels or truncation notices. No automatic newline for whole-file write, append or prepend. Empty ranged write deletes selected lines. Non-empty ranged writes preserve the old block's closing LF/CRLF if content lacks its final LF."},
					"old_text":{"type":"string","minLength":1,"description":"Required only for replace: exact non-empty case-sensitive text to match. Whitespace and LF/CRLF must agree. Matches may span lines but cannot cross the selected line range. When the target is known from instructions or context, try the replacement directly with an appropriate return and optional on_error read. No match fails the whole chain without changing the file."},
					"new_text":{"type":"string","description":"Required only for replace: literal replacement text. Empty deletes matches. Replacement content is not searched again; dollar signs/backslashes are not regex expressions."},
					"mode":{"type":"string","enum":["first","last","all"],"default":"first","description":"Only for replace: first (default) replaces the earliest match, last the latest, all every non-overlapping original match within the current selected range."},
					"start_line":{"type":"integer","minimum":1,"default":1,"description":"Only for read, write and replace: first line in the current staged file, 1-based inclusive. Omitted starts at 1. For write, omitting both line bounds replaces/creates the whole file; either bound requires existing lines."},
					"end_line":{"type":"integer","minimum":1,"description":"Only for read, write and replace: last inclusive line in the current staged file, at least start_line. Omitted means EOF. Read clamps an oversized end to EOF; edits reject it. A trailing LF adds no phantom line; an empty file has zero lines."},
					"start_byte":{"type":"integer","minimum":0,"default":0,"description":"Only for read: zero-based byte offset within start_line, excluding display labels. Omitted/0 starts at the line's beginning. Use the exact continuation offset supplied for a long line; it must address a byte inside the line and not split UTF-8. Later lines begin at byte 0."},
					"limit":{"type":"integer","minimum":1,"maximum":200,"default":100,"description":"Only for list: maximum direct directory entries, default 100, range 1-200. The shared 200-line/16-KiB output budget can reduce this. Hidden entries are included; symlinks are listed without following their targets."},
					"cursor":{"type":"string","default":"","maxLength":512,"description":"Only for list: opaque cursor from an earlier listing of this directory. Omitted/empty starts at the beginning. Entries are sorted by case-sensitive name. Do not construct cursors; concurrent directory changes can change subsequent pages."}
				}
			}},
			"return":{"type":"object","additionalProperties":false,"required":["type"],"description":"Required for any edit; optional for read/list-only calls. There is no automatic diff. Select additional output after all actions: summary, diff, read or list. Read/list actions already request their own output; avoid requesting the same content twice. All output previews share 200 lines/16 KiB including labels, plus bounded status and continuation notices.","properties":{
				"type":{"type":"string","enum":["summary","diff","read","list"],"description":"summary gives only compact status/change counts, plus any explicitly requested read/list actions. diff gives the net before/after diff and requires an edit. read gives final updated file text. list gives directory entries and requires list actions. No return on read/list-only calls means just the requested action output."},
				"context_lines":{"type":"integer","minimum":0,"maximum":100,"default":3,"description":"Only for diff: unchanged lines above/below each hunk. Default 3; 0 gives only changes; maximum 100. Diff comparisons over 256 KiB or 4000 combined lines and non-text content receive an omission notice; edits are never truncated."},
				"start_line":{"type":"integer","minimum":1,"default":1,"description":"Only for read: first final-file line to return after all actions, 1-based inclusive. Omitted starts at 1. Invalid ranges fail before committing edits."},
				"end_line":{"type":"integer","minimum":1,"description":"Only for read: last final-file line, inclusive and at least start_line. Omitted means EOF; oversized ends clamp to EOF; the shared preview budget still applies."},
				"start_byte":{"type":"integer","minimum":0,"default":0,"description":"Only for read: zero-based byte offset within start_line. Omitted/0 starts at its beginning. Use a supplied continuation cursor for long lines; never include display label bytes."},
				"limit":{"type":"integer","minimum":1,"maximum":200,"default":100,"description":"Only for list: maximum direct children, default 100, maximum 200, also subject to the shared output budget."},
				"cursor":{"type":"string","default":"","maxLength":512,"description":"Only for list: use an earlier opaque directory cursor; omitted/empty starts at the beginning."}
			}},
			"on_error":{"type":"object","additionalProperties":false,"required":["return"],"description":"Optional read-only diagnostic output for an execution failure on the same path. It never retries, edits, changes paths or hides the original failure. Failed mutation chains expose the original file snapshot, not rolled-back staged text. Cancellation and invalid inputs do not trigger recovery I/O.","properties":{
				"return":{"type":"object","additionalProperties":false,"required":["type"],"properties":{
					"type":{"type":"string","enum":["read","list"],"description":"read previews the unchanged file; list enumerates the target if it is a directory. An unavailable diagnostic is reported alongside the original error. The tool status remains error."},
					"start_line":{"type":"integer","minimum":1,"default":1,"description":"Only for read: first original-file line to inspect, 1-based inclusive; omitted means 1."},
					"end_line":{"type":"integer","minimum":1,"description":"Only for read: last inclusive original-file line, at least start_line. Omitted means EOF; oversized ends clamp to EOF."},
					"start_byte":{"type":"integer","minimum":0,"default":0,"description":"Only for read: byte offset within start_line, zero-based and default 0; use continuation values from earlier previews."},
					"limit":{"type":"integer","minimum":1,"maximum":200,"default":100,"description":"Only for list: maximum direct entries, default 100, maximum 200, within the preview budget."},
					"cursor":{"type":"string","default":"","maxLength":512,"description":"Only for list: opaque directory cursor; omitted/empty starts at the beginning."}
				}}
			}}
		},"required":["path","actions"]
	}`)
}
