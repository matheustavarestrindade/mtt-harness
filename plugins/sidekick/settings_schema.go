package sidekick

import (
	"encoding/json"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func configurationSchema() atom.Schema {
	return atom.Schema{JSON: json.RawMessage(`{
 "type":"object","additionalProperties":false,"description":"Sidekick workspace context assistance. Global defaults and partial workspace overrides are stored in the database; null removes an override. OFF by default. Enabling requires an explicitly configured provider-qualified worker model and at least one source. Background failures stay in plugin metrics. Workers are read-only and publish at a model-request boundary; they never start a user turn.",
 "properties":{
 "enabled":{"type":"boolean","default":false,"description":"Enable DOING-driven retrieval and context hints. Disabling cancels pending workers; previously delivered anchored notes remain until context removal."},
 "worker_model":{"type":"string","default":"","description":"Provider-qualified model ID for the filtering worker. Empty means no worker configured; required before enabling. Workspace allowlists apply."},
 "worker_effort":{"type":"string","default":"","description":"Reasoning effort supported by the selected worker model. Empty uses that model's default."},
 "memory_enabled":{"type":"boolean","default":true,"description":"Search current workspace memories at medium compression without changing their snapshot. Unavailable memory can be supplemented by enabled file retrieval."},
 "files_enabled":{"type":"boolean","default":true,"description":"Search regular UTF-8 files under the selected workspace using bounded lexical matching. No edits, commands, link traversal, or access outside that root. Hidden paths, common secret files, dependencies and build artifacts are excluded. Each search examines at most 4096 entries, 256 files, and 2 MiB; files larger than 64 KiB are skipped."},
 "debounce_ms":{"type":"integer","minimum":0,"maximum":10000,"default":750,"description":"Milliseconds to coalesce meaningful DOING changes before retrieval. Zero disables this delay; unchanged reaffirmations and TODO-only changes do not reset it."},
 "cooldown_ms":{"type":"integer","minimum":0,"maximum":300000,"default":15000,"description":"Minimum milliseconds between worker starts for one session. Zero disables the cooldown. Newer work replaces queued work during the wait."},
 "job_timeout_ms":{"type":"integer","minimum":1000,"maximum":300000,"default":45000,"description":"Maximum milliseconds for waiting, retrieval, verification and the worker request. Cancellation stops inference through the provider and fences late publication."},
 "worker_count":{"type":"integer","minimum":1,"maximum":4,"default":1,"description":"Maximum concurrent Sidekick retrieval/filter workers per workspace. At most 128 coalesced session workers can wait in one harness process."},
 "worker_output_tokens":{"type":"integer","minimum":128,"maximum":2048,"default":768,"description":"Maximum provider output tokens for one filtering call, including provider-counted reasoning. No model call occurs when no unseen sources are available."},
 "memory_limit":{"type":"integer","minimum":1,"maximum":12,"default":5,"description":"Maximum current memory records retrieved for a DOING query."},
 "memory_bytes":{"type":"integer","minimum":256,"maximum":16384,"default":6000,"description":"Combined UTF-8 byte budget for retrieved medium-compression memory text."},
 "file_limit":{"type":"integer","minimum":1,"maximum":12,"default":5,"description":"Maximum project-file excerpts returned for a DOING query."},
 "file_bytes":{"type":"integer","minimum":256,"maximum":16384,"default":8000,"description":"Combined UTF-8 byte budget for project-file excerpt text."},
 "source_bytes":{"type":"integer","minimum":256,"maximum":8000,"default":4000,"description":"UTF-8 byte bound for the latest user request and, separately, the bounded recent-context excerpt sent to the worker."},
 "hint_bytes":{"type":"integer","minimum":128,"maximum":4096,"default":1600,"description":"Maximum combined UTF-8 bytes of up to four useful, cited note texts. JSON quoting, scope labels and the sidekick wrapper add request bytes and must fit the model/context-policy budget."}
 }}`)}
}
