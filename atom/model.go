package atom

import (
	"encoding/json"
	"time"
)

type Prices struct {
	Currency          string
	Input             float64
	Output            float64
	CacheRead         float64
	CacheWrite        float64
	CacheReadUnknown  bool `json:",omitempty"`
	CacheWriteUnknown bool `json:",omitempty"`
	// Reasoning is an optional separate rate; nil uses the output rate.
	Reasoning *float64    `json:",omitempty"`
	Tiers     []PriceTier `json:",omitempty"`
	Source    string      `json:",omitempty"`
}

// PriceTier applies to the whole request when its input-token total exceeds
// AboveInputTokens. Input includes cache reads and cache writes.
type PriceTier struct {
	AboveInputTokens  int
	Input             float64
	Output            float64
	CacheRead         float64
	CacheWrite        float64
	CacheReadUnknown  bool     `json:",omitempty"`
	CacheWriteUnknown bool     `json:",omitempty"`
	Reasoning         *float64 `json:",omitempty"`
}

type Cost struct {
	Currency  string
	Value     float64
	Estimated bool `json:",omitempty"`
}

type ModelInfo struct {
	ID string
	// Name is an optional display label. Requests and allowlists use ID.
	Name   string `json:",omitempty"`
	Level  int
	Input  []MediaType
	Output []MediaType
	Tools  bool
	// ToolSupportUnknown distinguishes absent catalog metadata from explicit false.
	// Unknown support permits a tool request; the provider can reject it normally.
	ToolSupportUnknown     bool `json:",omitempty"`
	ContextMax             int
	Prices                 *Prices
	Reasoning              bool     `json:",omitempty"`
	ReasoningEfforts       []string `json:",omitempty"`
	DefaultReasoningEffort string   `json:",omitempty"`
	ReasoningSummary       string   `json:",omitempty"`
	Billing                string   `json:",omitempty"`
}

type ProviderSpec struct {
	Name             string
	Protocol         string
	Authentication   string
	APIURL           string
	ModelListURL     string
	ModelListFormat  string `json:",omitempty"`
	PriceTableURL    string
	Interval         time.Duration
	MetadataURL      string `json:",omitempty"`
	MetadataFormat   string `json:",omitempty"`
	MetadataProvider string `json:",omitempty"`
	Billing          string `json:",omitempty"`
}

type ProviderKey struct {
	Scope    string
	Provider string
	Key      string
}

type Usage struct {
	Input      int
	CacheRead  int
	CacheWrite int
	Output     int
	Reasoning  int
	Cost       *Cost
}

type UsageRecord struct {
	InstanceID string
	SessionID  SessionID
	ModelID    string
	Usage      Usage
	CreatedAt  time.Time
}

type Statistics struct {
	Calls      int
	Input      int
	CacheRead  int
	CacheWrite int
	Output     int
	Reasoning  int
	Cost       *Cost
	Costs      []Cost
}

func (statistics Statistics) CacheHitRate() float64 {
	full := statistics.Input + statistics.CacheRead + statistics.CacheWrite
	if full == 0 {
		return 0
	}
	return float64(statistics.CacheRead) / float64(full)
}

func (statistics Statistics) MarshalJSON() ([]byte, error) {
	type fields Statistics
	return json.Marshal(struct {
		fields
		CacheHitRate       float64
		CacheHitPercentage float64
	}{fields: fields(statistics), CacheHitRate: statistics.CacheHitRate(), CacheHitPercentage: statistics.CacheHitRate() * 100})
}

type Request struct {
	Model           string
	Messages        []Message
	Tools           []ToolSpec
	Params          map[string]any
	ReasoningEffort string `json:",omitempty"`
}

type ResponsePart struct {
	Text string
	// Reasoning contains only provider-exposed text or summaries, never encrypted state.
	Reasoning     string `json:",omitempty"`
	Content       []Content
	ToolIndex     *int
	ToolCall      *ToolCall
	Usage         *Usage
	ProviderState *ProviderState
}

// ProviderState preserves provider-specific reasoning items for stateless tool
// continuations. An adapter must only replay state belonging to its own provider.
type ProviderState struct {
	Provider  string
	Reasoning []json.RawMessage
	// ChatReasoning is replayed only to the originating chat-completions provider.
	ChatReasoning string `json:",omitempty"`
}
