package spacedrepetition

import (
	"encoding/json"
	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func configurationSchema() atom.Schema {
	return atom.Schema{JSON: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{
"enabled":{"type":["boolean","null"],"description":"Enable automatic instruction reminders and the configured recovery tool. Default false. Changes apply after active tool groups. Null removes this scope override."},
"interval":{"type":["object","null"],"additionalProperties":false,"description":"Context-growth interval. Nested fields inherit global/default values; null removes a nested override. The model capacity determines adaptive intervals, and reminders exclude their own text from growth.","properties":{
"mode":{"type":["string","null"],"enum":["tokens","model_fraction",null],"description":"tokens uses the fixed token value; model_fraction uses the selected model context capacity times fraction, capped by max_tokens. Default model_fraction."},
"tokens":{"type":["integer","null"],"minimum":256,"maximum":2097152,"description":"Fixed growth interval in input tokens for tokens mode. Default 32768; zero is invalid."},
"fraction":{"type":["number","null"],"minimum":0.01,"maximum":0.5,"description":"Fraction of model context capacity in model_fraction mode. Default 0.125 (12.5%); zero is invalid."},
"max_tokens":{"type":["integer","null"],"minimum":256,"maximum":2097152,"description":"Upper bound in input tokens for the adaptive interval. Default 32768; zero is invalid."}}},
"pattern":{"type":["array","null"],"minItems":1,"maxItems":32,"items":{"type":"string","enum":["low","medium"]},"description":"Repeating checkpoint levels, in order. Default [low,low,low,medium]. High is on demand through remember_instructions. Null restores inheritance."},
"worker_model":{"type":["string","null"],"description":"Provider-qualified model for high recovery. Empty disables the recovery tool while automatic reminders can still run. No model is chosen by default. Workspace allowlists apply."},
"worker_effort":{"type":["string","null"],"description":"Supported reasoning effort for the recovery model. Empty uses the model default. Null restores inheritance."},
"worker_output_tokens":{"type":["integer","null"],"minimum":256,"maximum":4096,"description":"Maximum provider output tokens per recovery model request. Default 1024; zero is invalid."},
"job_timeout_ms":{"type":["integer","null"],"minimum":1000,"maximum":600000,"description":"Total recovery deadline in milliseconds, including memory searches and model calls. Default 120000; zero is invalid. Session cancellation also stops the worker."},
"max_queries":{"type":["integer","null"],"minimum":1,"maximum":8,"description":"Maximum focused medium-compression memory queries per recovery. Default 4; zero is invalid."},
"memory_result_limit":{"type":["integer","null"],"minimum":1,"maximum":20,"description":"Maximum memory records retrieved per query before deduplication and the shared byte limit. Default 5."},
"memory_bytes":{"type":["integer","null"],"minimum":1024,"maximum":16384,"description":"Shared UTF-8 byte budget for medium-compression memory text per recovery. Default 12000. Whole records are selected without truncation."},
"source_bytes":{"type":["integer","null"],"minimum":1024,"maximum":16000,"description":"UTF-8 byte budget for recent user messages supplied to the recovery worker. Default 8000. Current reason and trusted prompt templates are separate inputs."},
"worker_count":{"type":["integer","null"],"minimum":1,"maximum":4,"description":"Maximum simultaneous recoveries in a workspace. Default 2. One session can have one active recovery; zero is invalid."}
}}`)}
}
