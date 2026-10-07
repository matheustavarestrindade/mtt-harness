package contextplugin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type memoryTool struct {
	plugin *Plugin
	name   string
}

func (tool *memoryTool) Name() string         { return tool.name }
func (tool *memoryTool) Categories() []string { return []string{"memory", "context"} }
func (tool *memoryTool) Check(context.Context, atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}
func (tool *memoryTool) Available(operationContext context.Context) (bool, error) {
	session, found := harness.SessionFrom(operationContext)
	if !found {
		return false, nil
	}
	configuration, operationError := tool.plugin.requestConfiguration(operationContext, session.InstanceID)
	if operationError != nil {
		return false, operationError
	}
	return configuration.Enabled && tool.plugin.unavailable == nil && tool.plugin.services.Embeddings != nil, nil
}

func (tool *memoryTool) Run(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	session, found := harness.SessionFrom(operationContext)
	if !found {
		return atom.ToolResult{}, fmt.Errorf("memory tool has no session context")
	}
	available, operationError := tool.Available(operationContext)
	if operationError != nil {
		return atom.ToolResult{}, operationError
	}
	if !available {
		return atom.ToolResult{}, fmt.Errorf("context memory is not enabled for this workspace")
	}
	var result any
	switch tool.name {
	case "ctx_drop":
		var input dropInput
		if operationError = json.Unmarshal(call.Input, &input); operationError == nil {
			result, operationError = tool.plugin.queueDrop(operationContext, session, input)
		}
	case "remember":
		var input rememberInput
		if operationError = json.Unmarshal(call.Input, &input); operationError == nil {
			result, operationError = tool.plugin.saveMemory(operationContext, session, call.ID, input)
		}
	case "ctx_wrapup":
		result, operationError = tool.plugin.requestWrapup(operationContext, session)
	case "search_memory":
		var input memorySearchInput
		if operationError = json.Unmarshal(call.Input, &input); operationError == nil {
			result, operationError = tool.plugin.searchMemory(operationContext, session, input)
		}
	case "list_memory_categories":
		result, operationError = tool.plugin.listCategories(operationContext, session.InstanceID)
	default:
		operationError = fmt.Errorf("memory operation %q is not registered", tool.name)
	}
	if operationError != nil {
		return atom.ToolResult{}, operationError
	}
	encoded, operationError := json.Marshal(result)
	if operationError != nil {
		return atom.ToolResult{}, operationError
	}
	return atom.ToolResult{CallID: call.ID, Status: atom.StatusOK, Content: []atom.Content{{Type: atom.Text, Text: string(encoded)}}}, nil
}

func (tool *memoryTool) Description() string {
	switch tool.name {
	case "ctx_drop":
		return "Queue currently loaded conversation messages for later context reduction. IDs come from the internal Context selection data at the request tail: messages lists [ID, role] pairs in conversation order, groups lists complete tool groups, and protected lists IDs that cannot be selected. These references are not file line labels or user-facing text. Select complete assistant tool-call/result groups; recent input, active work, system instructions and task state are protected. Messages remain in the next requests until ctx_wrapup or the configured forced checkpoint commits a reduction. remember=true prepares source-linked L/M/H memories and embeddings in the background; false archives only, without generating a summary. Both retain raw source content for workspace search. Preparation failures remove nothing. Categories label stored data but never appear in the injected memory text. This does not terminate a process, delete a file, erase conversation storage, or expand context immediately. Use one shared selection per coherent topic. The result reports preparation state, not a completed reduction."
	case "ctx_wrapup":
		return "Request a context checkpoint after this complete tool group, before the next model request. Only prepared reductions with durable archives and required memories can apply. Pending or failed work retains its messages. A successful reduction atomically selects a new context view and replaces the plain-text memory snapshot: fresh detail L, prior L to M, prior M to H, historian ideas I. A request that removes no context leaves the snapshot and levels unchanged. This tool returns before the next request boundary; it does not end the task or delete saved history. Choose a natural work breakpoint when possible; urgent context notices require a checkpoint."
	case "remember":
		return "Save text exactly as supplied in this workspace's persistent memory. Success means the note and its search index are saved and immediately queryable across sessions. No model rewrites, validates, or queues the note. Omit old_text to add a note; supply the exact text of one current memory to replace it, retaining previous versions. Missing or ambiguous replacement text changes nothing. Write the fact or decision itself, without narration about saving it or being asked to remember it. Reuse information already in context; a successful save does not need a follow-up memory query. This does not remove conversation messages or change files or provider settings."
	case "search_memory":
		return "Query this workspace's stored memories and retained conversation sources. Search starts from text or categories, not injected memory IDs. Category filters match any listed category before ranking. Text combines semantic similarity and literal matching. high is the shortest summary; medium and low provide more detail; raw returns original source text in bounded pages. Consolidated detail remains searchable normally. Superseded facts and deleted/reverted sources are hidden unless their explicit flags are set, and historical results are labelled. Results share a 16-KiB text budget, at most 20 hits. An opaque cursor continues one long raw result with the identical query and filters; it is workspace-bound. A query does not rewrite the cached memory snapshot; relevant detail can return at L at the next actual refresh."
	case "list_memory_categories":
		return "List category names used by current and consolidated memories in this workspace. This is metadata-only discovery and requires no model or embedding request. It does not list other workspaces, expose internal memory IDs, or modify the memory snapshot."
	}
	return ""
}

func (tool *memoryTool) InputSchema() atom.Schema {
	category := `{"type":"array","maxItems":16,"uniqueItems":true,"items":{"type":"string","pattern":"^[a-z0-9_-]{1,48}$"},"description":"Optional category names, at most 16. Lowercase letters, digits, underscore and hyphen; 1..48 bytes each. Empty uses automatic classification for new notes or no category filter for search."}`
	switch tool.name {
	case "ctx_drop":
		return atom.Schema{JSON: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["message_ids","remember"],"properties":{"message_ids":{"type":"array","minItems":1,"maxItems":128,"uniqueItems":true,"items":{"type":"integer","minimum":1},"description":"Stable positive context-message IDs visible in this session's current request. Select 1..128 IDs, including every member of a selected tool group."},"remember":{"type":"boolean","description":"Required. True creates searchable compressed memories before removal; false keeps only the searchable raw archive. Neither removes context immediately."},"categories":` + category + `}}`)}
	case "remember":
		return atom.Schema{JSON: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["text"],"properties":{"text":{"type":"string","minLength":1,"maxLength":8192,"description":"Exact note to save, nonblank and at most 8192 UTF-8 bytes. Whitespace, wording, and punctuation are preserved."},"old_text":{"type":"string","minLength":1,"maxLength":8192,"description":"Optional exact, case-sensitive text of one current memory to replace, at most 8192 UTF-8 bytes. Omit to add a note. A missing or ambiguous match fails without a change; do not guess text that is not available in context."},"categories":{"type":"array","maxItems":16,"uniqueItems":true,"items":{"type":"string","pattern":"^[a-z0-9_-]{1,48}$"},"description":"Optional category names; at most 16, each 1..48 lowercase ASCII letters, digits, underscores or hyphens. Omitted means no categories for a new note, or preserves categories for a replacement. An empty array explicitly clears categories. No automatic classification occurs."}}}`)}
	case "search_memory":
		return atom.Schema{JSON: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"query":{"type":"string","maxLength":2048,"description":"Natural-language question or literal identifier, up to 2048 UTF-8 bytes. Empty requires at least one category."},"categories":` + category + `,"compression":{"type":"string","enum":["high","medium","low","raw"],"default":"high","description":"Requested detail: high (shortest, default), medium, low (detailed summary), or raw (original sources)."},"source":{"type":"string","enum":["memories","archive","all"],"default":"memories","description":"Search compressed records, raw archives including archive-only reductions, or both. Default memories."},"include_deleted":{"type":"boolean","default":false,"description":"Include sources removed by session deletion or revert, and their retained memories. Default false. Results remain historical and do not restore live sessions."},"include_history":{"type":"boolean","default":false,"description":"Include superseded memory versions. Default false; consolidated current detail remains searchable without this flag."},"limit":{"type":"integer","minimum":1,"maximum":20,"default":5,"description":"Maximum result count, default 5, maximum 20. All results share a 16-KiB text budget."},"cursor":{"type":"string","maxLength":4096,"description":"Opaque continuation from a previous long raw result. Omit for a new search; reuse the same query, source, categories and history flags."}}}`)}
	default:
		return atom.Schema{JSON: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`)}
	}
}

func (tool *memoryTool) SearchDocument() string {
	switch tool.name {
	case "ctx_drop":
		return `Queue completed conversation work for deferred context reduction. For a decision worth retaining, use {"message_ids":[12,13],"remember":true,"categories":["architecture"]}. For disposable build output, select its complete tool group with remember=false. Preparation starts before the checkpoint and preserves the raw sources.`
	case "ctx_wrapup":
		return `Apply prepared context reductions at a work breakpoint. Call with {} after selecting context and allowing background preparation. This replaces the memory snapshot only if messages are actually removed, preserving cached text between refreshes.`
	case "remember":
		return `Save a note immediately without changing its text or removing conversation messages. Example: {"text":"The user's name is Matheus.","categories":["preferences"]}. Correct a known note with {"old_text":"The user's name is Matheus.","text":"The user's preferred name is Matt."}. Success needs no follow-up verification query.`
	case "search_memory":
		return `Recover workspace decisions and detailed conversation evidence. Example: {"query":"Why did we choose PostgreSQL?","compression":"low","categories":["decisions"]}. Use source=archive and compression=raw for original messages. include_deleted=true explicitly searches retained deleted conversation data.`
	case "list_memory_categories":
		return `Discover the category names available for workspace memory filtering. Call with {}. This reads category metadata without vector inference or a model request.`
	}
	return ""
}
